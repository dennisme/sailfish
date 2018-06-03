// Build tags: only build this for the simulation build. Be sure to note the required blank line after.
// +build ec

package obmc

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"

	eh "github.com/looplab/eventhorizon"
	"github.com/looplab/eventhorizon/utils"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	"github.com/superchalupa/go-redfish/src/log"
	"github.com/superchalupa/go-redfish/src/ocp/model"
	"github.com/superchalupa/go-redfish/src/ocp/root"
	"github.com/superchalupa/go-redfish/src/ocp/session"
	"github.com/superchalupa/go-redfish/src/ocp/stdcollections"
	"github.com/superchalupa/go-redfish/src/ocp/view"

	attr_prop "github.com/superchalupa/go-redfish/src/dell-resources/attribute-property"
	attr_res "github.com/superchalupa/go-redfish/src/dell-resources/attribute-resource"

	"github.com/superchalupa/go-redfish/src/dell-resources/ar_mapper"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/cmc.integrated"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/iom.slot"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.chassis"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.chassis/power"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.chassis/power/powersupply"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.chassis/thermal"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.chassis/thermal/fans"
	"github.com/superchalupa/go-redfish/src/dell-resources/chassis/system.modular"
	mgrCMCIntegrated "github.com/superchalupa/go-redfish/src/dell-resources/managers/cmc.integrated"
	"github.com/superchalupa/go-redfish/src/dell-resources/test"
)

type ocp struct {
	configChangeHandler func()
}

func (o *ocp) ConfigChangeHandler() { o.configChangeHandler() }

func makeUndef(name ...string) (ret []model.Option) {
	for _, n := range name {
		ret = append(ret, model.UpdateProperty(n, nil))
	}
	return
}

func New(ctx context.Context, logger log.Logger, cfgMgr *viper.Viper, viperMu *sync.Mutex, ch eh.CommandHandler, eb eh.EventBus, ew *utils.EventWaiter) *ocp {
	logger = logger.New("module", "ec")
	self := &ocp{}

	updateFns := []func(context.Context, *viper.Viper){}

	//
	// Create the (empty) model behind the /redfish/v1 service root. Nothing interesting here
	//
	// No Logger
	// No Model
	// No Controllers
	rootView := view.New(
		view.WithURI("/redfish/v1"),
	)
	domain.RegisterPlugin(func() domain.Plugin { return rootView })
	root.AddAggregate(ctx, rootView, ch, eb, ew)

	//*********************************************************************
	//  /redfish/v1/testview - a proof of concept test view and example
	//*********************************************************************
	// construction order:
	//   1) model
	//   2) controller(s) - pass model by args
	//   3) views - pass models and controllers by args
	//   4) aggregate - pass view
	//
	testLogger := logger.New("module", "testview")
	testModel := model.New(
		model.UpdateProperty("unique_name", "test_unique_name"),
		model.UpdateProperty("name", "name"),
		model.UpdateProperty("description", "description"),
		model.UpdateProperty("model", "model"),
	)
	testController, _ := ar_mapper.New(ctx, testLogger, testModel, "test/testview", ch, eb, ew)
	updateFns = append(updateFns, testController.ConfigChangedFn)
	testView := view.New(
		view.WithModel("default", testModel),
		view.WithController("ar_mapper", testController),
		view.WithURI("/redfish/v1/testview"),
	)
	domain.RegisterPlugin(func() domain.Plugin { return testView })
	test.AddAggregate(ctx, testView, ch)

	//*********************************************************************
	//  /redfish/v1/{Managers,Chassis,Systems,Accounts}
	//*********************************************************************
	// Add standard collections: Systems, Chassis, Mangers, Accounts
	//
	stdcollections.AddAggregate(ctx, rootView.GetUUID(), rootView.GetURI(), ch)

	//*********************************************************************
	// /redfish/v1/Sessions
	//*********************************************************************
	//
	sessionLogger := logger.New("module", "SessionService")
	sessionModel := model.New(
		model.UpdateProperty("session_timeout", 30))
	// the controller is what updates the model when ar entries change, also
	// handles patch from redfish
	sessionARMappingController, _ := ar_mapper.New(ctx, sessionLogger, sessionModel, "SessionService", ch, eb, ew)
	sessionView := view.New(
		view.WithModel("default", sessionModel),
		view.WithController("ar_mapper", sessionARMappingController),
		view.WithURI(rootView.GetURI()+"/SessionService"))
	domain.RegisterPlugin(func() domain.Plugin { return sessionView })
	session.AddAggregate(ctx, sessionView, rootView.GetUUID(), ch, eb, ew)

	//
	// Loop to create similarly named manager objects and the things attached there.
	//
	mgrLogger := logger.New("module", "Managers")
	var managers []*view.View
	for _, mgrName := range []string{
		"CMC.Integrated.1",
		"CMC.Integrated.2",
	} {
		//*********************************************************************
		//  /redfish/v1/Managers/CMC.Integrated.N
		//*********************************************************************
		mgrLogger := mgrLogger.New("module", "Managers/"+mgrName, "module", "Managers/CMC.Integrated")
		mdl, _ := mgrCMCIntegrated.New(
			mgrCMCIntegrated.WithUniqueName(mgrName),
			model.UpdateProperty("name", nil),
			model.UpdateProperty("description", nil),
			model.UpdateProperty("model", nil),
			model.UpdateProperty("timezone", nil),
			model.UpdateProperty("firmware_version", nil),
			model.UpdateProperty("health_state", nil),
			model.UpdateProperty("redundancy_health_state", nil),
			model.UpdateProperty("redundancy_mode", nil),
			model.UpdateProperty("redundancy_min", nil),
			model.UpdateProperty("redundancy_max", nil),
			model.UpdateProperty("attributes", map[string]map[string]map[string]interface{}{}),
		)
		// AR -> model properties and Redfish PATCH -> AR
		armapper, _ := ar_mapper.New(ctx, mgrLogger, mdl, "Managers/"+mgrName, ch, eb, ew)

		// populate 'attributes' property with AR entries matching this FQDD ('mgrName')
		ardumper, _ := attr_prop.NewController(ctx, mdl, []string{mgrName}, ch, eb, ew)

		// let the controller re-read its mappings when config file changes... nifty
		updateFns = append(updateFns, armapper.ConfigChangedFn)

		// add the actual view
		vw := view.New(
			view.WithURI("/redfish/v1/Managers/"+mgrName),
			view.WithModel("default", mdl),
			view.WithController("ar_mapper", armapper),
		)
		domain.RegisterPlugin(func() domain.Plugin { return vw })

		mgrView := mgrCMCIntegrated.AddAggregate(ctx, mgrLogger, vw, ch, eb, ew)

		// need these views later to get the URI/UUID
		managers = append(managers, mgrView)

		// Create the .../Attributes URI. Attributes are stored in the attributes property
		v := attr_prop.NewView(ctx, mdl, ardumper)
		mgrUUID := attr_res.AddView(ctx, "/redfish/v1/Managers/"+mgrName+"/Attributes", mgrName+".Attributes", ch, eb, ew)
		attr_prop.EnhanceExistingUUID(ctx, v, ch, mgrUUID)

		//*********************************************************************
		// Create CHASSIS objects for CMC.Integrated.N
		//*********************************************************************
		chasLogger := logger.New("module", "Chassis/"+mgrName, "module", "Chassis/CMC.Integrated")
		chasModel, _ := generic_chassis.New(
			mgrCMCIntegrated.WithUniqueName(mgrName),
			model.UpdateProperty("asset_tag", ""),
			model.UpdateProperty("serial", ""),
			model.UpdateProperty("part_number", ""),
			model.UpdateProperty("chassis_type", ""),
			model.UpdateProperty("model", ""),
			model.UpdateProperty("manufacturer", ""),
			model.UpdateProperty("name", ""),
			model.UpdateProperty("attributes", map[string]map[string]map[string]interface{}{}),
		)
		// the controller is what updates the model when ar entries change,
		// also handles patch from redfish
		chasController, _ := ar_mapper.New(ctx, chasLogger, chasModel, "Managers/"+mgrName, ch, eb, ew)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('mgrName')
		chasArdump, _ := attr_prop.NewController(ctx, chasModel, []string{mgrName}, ch, eb, ew)

		// let the controller re-read its mappings when config file changes... nifty
		updateFns = append(updateFns, chasController.ConfigChangedFn)

		// add the aggregate to the view tree
		cmc_chassis.AddView(ctx, chasLogger, chasModel, chasController, ch, eb, ew)

		// Create the .../Attributes URI. Attributes are stored in the attributes property of the chasModel
		v2 := attr_prop.NewView(ctx, chasModel, chasArdump)
		chasUUID := attr_res.AddView(ctx, "/redfish/v1/Chassis/"+mgrName+"/Attributes", mgrName+".Attributes", ch, eb, ew)
		attr_prop.EnhanceExistingUUID(ctx, v2, ch, chasUUID)
	}

	chasName := "System.Chassis.1"
	chasLogger := logger.New("module", "Chassis/"+chasName, "module", "Chassis/System.Chassis")
	{
		// ************************************************************************
		// CHASSIS System.Chassis.1
		// ************************************************************************
		chasModel, _ := generic_chassis.New(
			mgrCMCIntegrated.WithUniqueName(chasName),
			generic_chassis.AddManagedBy(managers[0].GetURI()),
			model.UpdateProperty("asset_tag", ""),
			model.UpdateProperty("serial", ""),
			model.UpdateProperty("part_number", ""),
			model.UpdateProperty("chassis_type", ""),
			model.UpdateProperty("model", ""),
			model.UpdateProperty("manufacturer", ""),
			model.UpdateProperty("name", ""),
			model.UpdateProperty("description", ""),
			model.UpdateProperty("power_state", ""),
			model.UpdateProperty("attributes", map[string]map[string]map[string]interface{}{}),
		)
		// the controller is what updates the model when ar entries change,
		// also handles patch from redfish
		chasController, _ := ar_mapper.New(ctx, chasLogger, chasModel, "Chassis/"+chasName, ch, eb, ew)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('chasName')
		chasArdump, _ := attr_prop.NewController(ctx, chasModel, []string{chasName}, ch, eb, ew)

		// let the controller re-read its mappings when config file changes... nifty
		updateFns = append(updateFns, chasController.ConfigChangedFn)

		system_chassis.AddView(ctx, chasLogger, chasModel, chasController, ch, eb, ew)

		// Create the .../Attributes URI. Attributes are stored in the attributes property of the chasModel
		v2 := attr_prop.NewView(ctx, chasModel, chasArdump)
		chasUUID := attr_res.AddView(ctx, "/redfish/v1/Chassis/"+chasName+"/Attributes", chasName+".Attributes", ch, eb, ew)
		attr_prop.EnhanceExistingUUID(ctx, v2, ch, chasUUID)
	}

	//*********************************************************************
	// Create Power objects for System.Chassis.1
	//*********************************************************************
	powerLogger := chasLogger.New("module", "Chassis/System.Chassis/Power")

	powerModel := model.New(
		mgrCMCIntegrated.WithUniqueName("Power"),
		model.UpdateProperty("power_supply_views", []interface{}{}),
	)
	// the controller is what updates the model when ar entries change,
	// also handles patch from redfish
	powerController, _ := ar_mapper.New(ctx, powerLogger, powerModel, "Chassis/"+chasName+"/Power", ch, eb, ew)
	power.AddView(ctx, powerLogger, chasName, powerModel, powerController, ch, eb, ew)

	psu_views := []interface{}{}
	for _, psuName := range []string{
		"PSU.Slot.1", "PSU.Slot.2", "PSU.Slot.3",
		"PSU.Slot.4", "PSU.Slot.5", "PSU.Slot.6",
	} {
		psuLogger := powerLogger.New("module", "Chassis/System.Chassis/Power/PowerSupply")

		psuModel := model.New(
			model.UpdateProperty("unique_name", psuName),
			model.UpdateProperty("unique_id", psuName),
			model.UpdateProperty("name", psuName),
			model.UpdateProperty("capacity_watts", "INVALID"),
			model.UpdateProperty("firmware_version", "NOT INVENTORIED"),
			model.UpdateProperty("component_id", "INVALID"),
			model.UpdateProperty("line_input_voltage", ""),
			model.UpdateProperty("input_current", ""),
			model.UpdateProperty("attributes", map[string]map[string]map[string]interface{}{}),
		)
		// the controller is what updates the model when ar entries change,
		// also handles patch from redfish
		psuController, _ := ar_mapper.New(ctx, psuLogger, psuModel, "PowerSupply/"+psuName, ch, eb, ew)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('psuName')
		psuARdump, _ := attr_prop.NewController(ctx, psuModel, []string{psuName}, ch, eb, ew)

		// let the controller re-read its mappings when config file changes... nifty
		updateFns = append(updateFns, psuController.ConfigChangedFn)

		attributeView := attr_prop.NewView(ctx, psuModel, psuARdump)
		_, psu := powersupply.NewView(ctx, psuLogger, chasName, psuName, psuModel, attributeView, psuController, psuARdump, ch, eb, ew)

		p := &domain.RedfishResourceProperty{}
		p.Parse(psu)
		psu_views = append(psu_views, p)
	}
	powerModel.ApplyOption(model.UpdateProperty("power_supply_views", &domain.RedfishResourceProperty{Value: psu_views}))

	//*********************************************************************
	// Create Thermal objects for System.Chassis.1
	//*********************************************************************
	thermalLogger := chasLogger.New("module", "Chassis/System.Chassis/Thermal")

	thermalModel := model.New(
		mgrCMCIntegrated.WithUniqueName("Thermal"),
		model.UpdateProperty("fan_views", []interface{}{}),
		model.UpdateProperty("thermal_views", []interface{}{}),
	)
	// the controller is what updates the model when ar entries change,
	// also handles patch from redfish
	thermalARMapper, _ := ar_mapper.New(ctx, thermalLogger, thermalModel, "Chassis/"+chasName+"/Thermal", ch, eb, ew)

	thermalView := view.New(
		view.WithURI("/redfish/v1/Chassis/"+chasName+"/Thermal"),
		view.WithModel("default", thermalModel),
		view.WithController("ar_mapper", thermalARMapper),
	)
	domain.RegisterPlugin(func() domain.Plugin { return thermalView })
	thermal.AddView(ctx, thermalLogger, thermalView, ch, eb, ew)

	fan_views := []interface{}{}
	for _, fanName := range []string{
		"Fan.Slot.1", "Fan.Slot.2", "Fan.Slot.3",
		"Fan.Slot.4", "Fan.Slot.5", "Fan.Slot.6",
		"Fan.Slot.7", "Fan.Slot.8", "Fan.Slot.9",
	} {
		fanLogger := chasLogger.New("module", "Chassis/System.Chassis/Thermal/Fan")

		fanModel := model.New(
			model.UpdateProperty("unique_id", fanName),
			model.UpdateProperty("name", "UNSET"),
			model.UpdateProperty("firmware_version", "UNSET"),
			model.UpdateProperty("hardware_version", "UNSET"),
			model.UpdateProperty("reading", "UNSET"),
			model.UpdateProperty("reading_units", "UNSET"),
			model.UpdateProperty("oem_reading", "UNSET"),
			model.UpdateProperty("oem_reading_units", "UNSET"),
			model.UpdateProperty("graphics_uri", "UNSET"),
			model.UpdateProperty("attributes", map[string]map[string]map[string]interface{}{}),
		)
		// the controller is what updates the model when ar entries change,
		// also handles patch from redfish
		fanController, _ := ar_mapper.New(ctx, fanLogger, fanModel, "Fans/"+fanName, ch, eb, ew)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('fanName')
		fanARdump, _ := attr_prop.NewController(ctx, fanModel, []string{fanName}, ch, eb, ew)

		// let the controller re-read its mappings when config file changes... nifty
		updateFns = append(updateFns, fanController.ConfigChangedFn)

		v := view.New(
			view.WithURI("/redfish/v1/Chassis/"+chasName+"/Sensors/Fans/"+fanName),
			view.WithModel("default", fanModel),
			view.WithController("ar_mapper", fanController),
			view.WithController("ar_dumper", fanARdump),
			view.WithFormatter("attributeFormatter", attr_prop.FormatAttributeDump),
		)
		domain.RegisterPlugin(func() domain.Plugin { return v })
		fanFragment := fans.AddView(ctx, fanLogger, v, ch, eb, ew)

		p := &domain.RedfishResourceProperty{}
		p.Parse(fanFragment)
		fan_views = append(fan_views, p)
	}
	thermalModel.ApplyOption(model.UpdateProperty("fan_views", &domain.RedfishResourceProperty{Value: fan_views}))

	// ************************************************************************
	// CHASSIS IOM.Slot
	// ************************************************************************
	for _, iomName := range []string{
		"IOM.Slot.A1", "IOM.Slot.A1a", "IOM.Slot.A1b",
		"IOM.Slot.A2", "IOM.Slot.A2a", "IOM.Slot.A2b",
		"IOM.Slot.B1", "IOM.Slot.B1a", "IOM.Slot.B1b",
		"IOM.Slot.B2", "IOM.Slot.B2a", "IOM.Slot.B2b",
		"IOM.Slot.C1",
		"IOM.Slot.C2",
	} {
		iomLogger := logger.New("module", "Chassis/"+iomName, "module", "Chassis/IOM.Slot")
		iomModel, _ := generic_chassis.New(
			generic_chassis.WithUniqueName(iomName),
			generic_chassis.AddManagedBy(managers[0].GetURI()),
			model.UpdateProperty("service_tag", ""),
			model.UpdateProperty("asset_tag", ""),
			model.UpdateProperty("description", ""),
			model.UpdateProperty("power_state", ""),
			model.UpdateProperty("serial", ""),
			model.UpdateProperty("part_number", ""),
			model.UpdateProperty("chassis_type", ""),
			model.UpdateProperty("model", ""),
			model.UpdateProperty("name", ""),
			model.UpdateProperty("manufacturer", ""),
		)
		// the controller is what updates the model when ar entries change,
		// also handles patch from redfish
		iomController, _ := ar_mapper.New(ctx, iomLogger, iomModel, "Mangaers/"+iomName, ch, eb, ew)
		updateFns = append(updateFns, iomController.ConfigChangedFn)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('iomName')
		iomARdump, _ := attr_prop.NewController(ctx, iomModel, []string{iomName}, ch, eb, ew)

		iomView := view.New(
			view.WithURI("/redfish/v1/Chassis/"+iomName),
			view.WithModel("default", iomModel),
			view.WithController("ar_mapper", iomController),
			view.WithController("ar_dumper", iomARdump),
			view.WithFormatter("attributeFormatter", attr_prop.FormatAttributeDump),
		)
		domain.RegisterPlugin(func() domain.Plugin { return iomView })
		iom_chassis.AddAggregate(ctx, iomLogger, iomView, ch, eb, ew)

		// Create the .../Attributes URI. Attributes are stored in the attributes property of the iomModel
		v2 := attr_prop.NewView(ctx, iomModel, iomARdump)
		iom := attr_res.AddView(ctx, "/redfish/v1/Chassis/"+iomName+"/Attributes", iomName+".Attributes", ch, eb, ew)
		attr_prop.EnhanceExistingUUID(ctx, v2, ch, iom)
	}

	for _, sledName := range []string{
		"System.Modular.1", "System.Modular.1a", "System.Modular.1b",
		"System.Modular.2", "System.Modular.2a", "System.Modular.2b",
		"System.Modular.3", "System.Modular.3a", "System.Modular.3b",
		"System.Modular.4", "System.Modular.4a", "System.Modular.4b",
		"System.Modular.5", "System.Modular.5a", "System.Modular.5b",
		"System.Modular.6", "System.Modular.6a", "System.Modular.6b",
		"System.Modular.7", "System.Modular.7a", "System.Modular.7b",
		"System.Modular.8", "System.Modular.8a", "System.Modular.8b",
	} {
		sledLogger := logger.New("module", "Chassis/System.Modular", "module", "Chassis/"+sledName)
		sledModel, _ := generic_chassis.New(
			generic_chassis.WithUniqueName(sledName),
			generic_chassis.AddManagedBy(managers[0].GetURI()),
			model.UpdateProperty("service_tag", ""),
			model.UpdateProperty("power_state", ""),
			model.UpdateProperty("chassis_type", ""),
			model.UpdateProperty("model", ""),
			model.UpdateProperty("manufacturer", ""),
			model.UpdateProperty("serial", ""),
		)
		sledController, _ := ar_mapper.New(ctx, sledLogger, sledModel, "Chassis/"+sledName, ch, eb, ew)
		updateFns = append(updateFns, sledController.ConfigChangedFn)

		// This controller will populate 'attributes' property with AR entries matching this FQDD ('sledName')
		sledARdump, _ := attr_prop.NewController(ctx, sledModel, []string{sledName}, ch, eb, ew)

		sledView := view.New(
			view.WithURI("/redfish/v1/Chassis/"+sledName),
			view.WithModel("default", sledModel),
			view.WithController("ar_mapper", sledController),
			view.WithController("ar_dumper", sledARdump),
			view.WithFormatter("attributeFormatter", attr_prop.FormatAttributeDump),
		)
		domain.RegisterPlugin(func() domain.Plugin { return sledView })
		sled_chassis.AddAggregate(ctx, sledLogger, sledView, ch, eb, ew)

		// Create the .../Attributes URI. Attributes are stored in the attributes property of the sledModel
		v2 := attr_prop.NewView(ctx, sledModel, sledARdump)
		sled := attr_res.AddView(ctx, "/redfish/v1/Chassis/"+sledName+"/Attributes", sledName+".Attributes", ch, eb, ew)
		attr_prop.EnhanceExistingUUID(ctx, v2, ch, sled)
	}

	// VIPER Config:
	// pull the config from the YAML file to populate some static config options
	self.configChangeHandler = func() {
		logger.Info("Re-applying configuration from config file.")
		sessionModel.ApplyOption(model.UpdateProperty("session_timeout", cfgMgr.GetInt("session.timeout")))

		for _, fn := range updateFns {
			fn(ctx, cfgMgr)
		}
	}
	self.ConfigChangeHandler()

	cfgMgr.SetDefault("main.dumpConfigChanges.filename", "redfish-changed.yaml")
	cfgMgr.SetDefault("main.dumpConfigChanges.enabled", "true")
	dumpViperConfig := func() {
		viperMu.Lock()
		defer viperMu.Unlock()

		dumpFileName := cfgMgr.GetString("main.dumpConfigChanges.filename")
		enabled := cfgMgr.GetBool("main.dumpConfigChanges.enabled")
		if !enabled {
			return
		}

		// TODO: change this to a streaming write (reduce mem usage)
		var config map[string]interface{}
		cfgMgr.Unmarshal(&config)
		output, _ := yaml.Marshal(config)
		_ = ioutil.WriteFile(dumpFileName, output, 0644)
	}

	// Well, until observer stuff re-implemented, this can't be triggered, so
	// let's keep it around to remind us to reimplement it but keep compiler
	// happy
	_ = dumpViperConfig

	/*
	   // TODO: reimplement observer pattern on the model
	   //
	   //	sessionModel.AddPropertyObserver("session_timeout", func(newval interface{}) {
	   //		viperMu.Lock()
	   //		cfgMgr.Set("session.timeout", newval.(int))
	   //		viperMu.Unlock()
	   //		dumpViperConfig()
	   //	})
	*/

	return self
}

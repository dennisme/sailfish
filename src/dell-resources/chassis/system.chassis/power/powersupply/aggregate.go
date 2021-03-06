package powersupply

import (
	"context"

	"github.com/superchalupa/go-redfish/src/log"
	"github.com/superchalupa/go-redfish/src/ocp/view"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	eh "github.com/looplab/eventhorizon"
)

// So... this class is set up in a somewhat interesting way to support having
// PSU.Slot.N objects both as PowerSupplies/PSU.Slot.N as well as in the main
// Power object.

func AddAggregate(ctx context.Context, logger log.Logger, v *view.View, ch eh.CommandHandler) map[string]interface{} {
	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          v.GetUUID(),
			Collection:  false,
			ResourceURI: v.GetURI(),
			Type:        "#Power.v1_0_2.PowerSupply",
			Context:     "/redfish/v1/$metadata#Power.PowerSystem.Chassis.1/Power/$entity",
			Privileges: map[string]interface{}{
				"GET":    []string{"Login"},
				"POST":   []string{}, // cannot create sub objects
				"PUT":    []string{},
				"PATCH":  []string{"ConfigureManager"},
				"DELETE": []string{}, // can't be deleted
			},
			Properties: getViewFragment(v),
		})

	return getViewFragment(v)
}

//
// this view fragment can be attached elsewhere in the tree
//
func getViewFragment(v *view.View) map[string]interface{} {
	properties := map[string]interface{}{
		"@odata.type":             "#Power.v1_0_2.PowerSupply",
		"@odata.context":          "/redfish/v1/$metadata#Power.PowerSystem.Chassis.1/Power/$entity",
		"@odata.id":               v.GetURI(),
		"Name@meta":               v.Meta(view.PropGET("name")),
		"MemberId@meta":           v.Meta(view.PropGET("unique_id")),
		"PowerCapacityWatts@meta": v.Meta(view.PropGET("capacity_watts")),
		"LineInputVoltage@meta":   v.Meta(view.PropGET("line_input_voltage")), //TODO
		"FirmwareVersion@meta":    v.Meta(view.PropGET("firmware_version")),

		"Status": map[string]interface{}{
			"HealthRollup@meta": v.Meta(view.GETProperty("psu_rollup"), view.GETModel("global_health")),
			"State@meta":        v.Meta(view.PropGET("state")),
			"Health@meta":       v.Meta(view.GETProperty("psu_rollup"), view.GETModel("global_health")),
		},

		"Oem": map[string]interface{}{
			"Dell": map[string]interface{}{
				"@odata.type":       "#DellPower.v1_0_0.DellPowerSupply",
				"ComponentID@meta":  v.Meta(view.PropGET("component_id")),
				"InputCurrent@meta": v.Meta(view.PropGET("input_current")), //TODO
				"Attributes@meta":   v.Meta(view.GETProperty("attributes"), view.GETFormatter("attributeFormatter"), view.GETModel("default"), view.PropPATCH("attributes", "ar_dump")),
			},
		},
	}

	properties["Redundancy"] = []interface{}{}
	properties["Redundancy@odata.count"] = len(properties["Redundancy"].([]interface{}))

	return properties
}

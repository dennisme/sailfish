package sled_chassis

import (
	"context"

	"github.com/superchalupa/go-redfish/src/log"
	"github.com/superchalupa/go-redfish/src/ocp/view"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	eh "github.com/looplab/eventhorizon"
)

func AddAggregate(ctx context.Context, logger log.Logger, v *view.View, ch eh.CommandHandler, eb eh.EventBus) {
	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          v.GetUUID(),
			Collection:  false,
			ResourceURI: v.GetURI(),
			Type:        "#Chassis.v1_0_2.Chassis",
			Context:     "/redfish/v1/$metadata#ChassisCollection.ChassisCollection/Members/$entity",
			Privileges: map[string]interface{}{
				"GET":    []string{"Login"},
				"POST":   []string{}, // cannot create sub objects
				"PUT":    []string{},
				"PATCH":  []string{"ConfigureManager"},
				"DELETE": []string{}, // can't be deleted
			},
			Properties: map[string]interface{}{
				"Id@meta":           v.Meta(view.GETProperty("unique_name"), view.GETModel("default")),
				"SKU@meta":          v.Meta(view.GETProperty("service_tag"), view.GETModel("default")),
				"PowerState@meta":   v.Meta(view.GETProperty("power_state"), view.GETModel("default")),
				"ChassisType@meta":  v.Meta(view.GETProperty("chassis_type"), view.GETModel("default")),
				"Model@meta":        v.Meta(view.GETProperty("model"), view.GETModel("default")),
				"Manufacturer@meta": v.Meta(view.GETProperty("manufacturer"), view.GETModel("default")),
				"SerialNumber@meta": v.Meta(view.GETProperty("serial"), view.GETModel("default")),
				"Description@meta":  v.Meta(view.GETProperty("description"), view.GETModel("default")),

				"Links": map[string]interface{}{
					"ManagedBy@meta":             v.Meta(view.GETProperty("managed_by"), view.GETModel("default")),
					"ManagedBy@odata.count@meta": v.Meta(view.GETProperty("managed_by_count")),
				},

				"Status": map[string]interface{}{
					"HealthRollup@meta": v.Meta(view.GETProperty("sled_rollup"), view.GETModel("global_health")),
					"State":             "Enabled", //hardcoded
					"Health@meta":       v.Meta(view.PropGET("health")),
				},
				"PartNumber@meta": v.Meta(view.GETProperty("part_number"), view.GETModel("default")),
				"Name@meta":       v.Meta(view.GETProperty("name"), view.GETModel("default")),
				"Oem": map[string]interface{}{
					"OemChassis": map[string]interface{}{
						"@odata.id": v.GetURI() + "/Attributes",
					},
				},
				"Actions": map[string]interface{}{
					"Oem": map[string]interface{}{
						// TODO: Remove per JIT-66996
						"#DellChassis.v1_0_0#DellChassis.PeripheralMapping": map[string]interface{}{
							"MappingType@Redfish.AllowableValues": []string{
								"Accept",
								"Clear",
							},
							"target": v.GetActionURI("chassis.peripheralmapping"),
						},
						"#Chassis.VirtualReseat": map[string]interface{}{
							"target": v.GetActionURI("sledvirtualreseat"),
						},
						"#DellChassis.v1_0_0.PeripheralMapping": map[string]interface{}{
							"MappingType@Redfish.AllowableValues": []string{
								"Accept",
								"Clear",
							},
							"target": v.GetActionURI("chassis.peripiheralmapping"),
						},
						"#DellChassis.v1_0_0.VirtualReseat": map[string]interface{}{
							"target": v.GetActionURI("chassis.sledvirtualreseat"),
						},
					},
				},
			}})
}

package thermal

import (
	"context"

	"github.com/superchalupa/go-redfish/src/log"
	"github.com/superchalupa/go-redfish/src/ocp/view"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	eh "github.com/looplab/eventhorizon"
)

func AddAggregate(ctx context.Context, logger log.Logger, v *view.View, ch eh.CommandHandler) {
	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          v.GetUUID(),
			Collection:  false,
			ResourceURI: v.GetURI(),
			Type:        "#Thermal.v1_0_2.Thermal",
			Context:     "/redfish/v1/$metadata#Thermal.Thermal",
			Privileges: map[string]interface{}{
				"GET":    []string{"Login"},
				"POST":   []string{}, // cannot create sub objects
				"PUT":    []string{},
				"PATCH":  []string{"ConfigureManager"},
				"DELETE": []string{}, // can't be deleted
			},
			Properties: map[string]interface{}{
				"Id":                            "Thermal",
				"Name":                          "Thermal",
				"Description":                   "Represents the properties for Temperature and Cooling",
				"Fans@meta":                     v.Meta(view.PropGET("fan_views")),
				"Fans@odata.count@meta":         v.Meta(view.PropGET("fan_views_count")),
				"Temperatures@meta":             v.Meta(view.PropGET("thermal_views")),       //TODO: fix this in ec.go
				"Temperatures@odata.count@meta": v.Meta(view.PropGET("thermal_views_count")), //TODO
				"Oem": map[string]interface{}{
					"EID_674": map[string]interface{}{
						"FansSummary": map[string]interface{}{
							"Status": map[string]interface{}{
								"HealthRollup@meta": v.Meta(view.GETProperty("fan_rollup"), view.GETModel("global_health")),
								"Health@meta":       v.Meta(view.GETProperty("fan_rollup"), view.GETModel("global_health")),
							},
						},
						"TemperaturesSummary": map[string]interface{}{
							"Status": map[string]interface{}{
								"HealthRollup@meta": v.Meta(view.GETProperty("temperature_rollup"), view.GETModel("global_health")),
								"Health@meta":       v.Meta(view.GETProperty("temperature_rollup"), view.GETModel("global_health")),
							},
						},
					},
				},
				"Redundancy@meta":             v.Meta(view.PropGET("redundancy_views")),       //TODO: should something be here? this is empty in odatalite...
				"Redundancy@odata.count@meta": v.Meta(view.PropGET("redundancy_views_count")), //TODO
			}})
}

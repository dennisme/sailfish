package test

// this file should define the BMC Manager object golang data structures where
// we put all the data, plus the aggregate that pulls the data.  actual data
// population should happen in an impl class. ie. no dbus calls in this file

import (
	"context"
    "fmt"

	"github.com/superchalupa/go-redfish/src/ocp/model"
	"github.com/superchalupa/go-redfish/src/ocp/view"
	"github.com/superchalupa/go-redfish/src/dell-resources/ar_mapper"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	eh "github.com/looplab/eventhorizon"
)

func NewView(ctx context.Context, s *model.Model, c *ar_mapper.ARMappingController, ch eh.CommandHandler) *view.View {

	v := view.NewView(
			view.MakeUUID(),
			view.WithModel(s),
            view.WithNamedController("ar_mapper", c),
            view.WithUniqueName( fmt.Sprintf("%v", eh.NewUUID()) ),
		)

	domain.RegisterPlugin(func() domain.Plugin { return v })

	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          v.GetUUID(),
			Collection:  false,
			ResourceURI: "/redfish/v1/testview",
			Type:        "#Manager.v1_0_2.Manager",
			Context:     "/redfish/v1/$metadata#Manager.Manager",
			Privileges: map[string]interface{}{
				"GET":    []string{"Login"},
				"POST":   []string{}, // cannot create sub objects
				"PUT":    []string{},
				"PATCH":  []string{"ConfigureManager"},
				"DELETE": []string{}, // can't be deleted
			},
			Properties: map[string]interface{}{
				"Id":               s.GetProperty("unique_name").(string),
				"Name@meta":        v.Meta(view.PropGET("name")),
				"Description@meta": v.Meta(view.PropGET("description")),
				"Model@meta":       v.Meta(view.PropGET("model"), view.PropPATCH("model", "ar_mapper")),
			}})

	return v
}
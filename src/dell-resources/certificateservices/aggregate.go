package certificateservices

import (
	"context"

	"github.com/superchalupa/go-redfish/src/ocp/view"
	domain "github.com/superchalupa/go-redfish/src/redfishresource"

	eh "github.com/looplab/eventhorizon"
)

func AddAggregate(ctx context.Context, v *view.View, baseUri string, ch eh.CommandHandler) (ret eh.UUID) {
	ret = eh.NewUUID()

	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          ret,
			Collection:  false,
			ResourceURI: baseUri + "/CertificateService",
			Type:        "#DellCertificateService.v1_0_0.DellCertificateService",
			Context:     "/redfish/v1/$metadata#DellCertificateService.DellCertificateService",
			Privileges: map[string]interface{}{
				"GET":    []string{"ConfigureManager"},
				"POST":   []string{},
				"PUT":    []string{},
				"PATCH":  []string{},
				"DELETE": []string{},
			},
			Properties: map[string]interface{}{
				"Id":                             "CertificateService",
				"Name":                           "Certificate Service",
				"Description":                    "Represents the properties of Certificate Service",
				"CertificateSigningRequest@meta": v.Meta(view.PropGET("certificate_signing_request")),
				"Actions": map[string]interface{}{
					"#DellCertificateService.GenerateCSR": map[string]interface{}{
						"target": v.GetActionURI("certificates.generatecsr"),
					},
				},
				"CertificateInventory": map[string]interface{}{
					"@odata.id": v.GetURI() + "CertificateService/CertificateInventory",
				},
			}})

	ch.HandleCommand(
		ctx,
		&domain.CreateRedfishResource{
			ID:          eh.NewUUID(),
			Collection:  true,
			ResourceURI: baseUri + "/CertificateService/CertificateInventory",
			Type:        "#DellCertificateInventoryCollection.DellCertificateInventoryCollection",
			Context:     "/redfish/v1/$metadata#DellCertificateInventoryCollection.DellCertificateInventoryCollection",
			Privileges: map[string]interface{}{
				"GET":    []string{"ConfigureManager"},
				"POST":   []string{},
				"PUT":    []string{},
				"PATCH":  []string{},
				"DELETE": []string{},
			},
			Properties: map[string]interface{}{
				"Name":        "Certificate Inventory Collection",
				"Description": "Collection of Certificate Inventory",
			}})

	return
}

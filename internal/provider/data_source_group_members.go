package googleworkspace

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
)

func dataSourceGroupMembers() *schema.Resource {
	// Generate datasource schema from resource
	dsSchema := datasourceSchemaFromResourceSchema(resourceGroupMembers().Schema)
	addRequiredFieldsToSchema(dsSchema, "group_id")
	//addExactlyOneOfFieldsToSchema(dsSchema, "member_id", "email")

	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Group Members data source in the Terraform Googleworkspace provider.",

		ReadContext: dataSourceGroupMembersRead,

		Schema: dsSchema,
	}
}

func dataSourceGroupMembersRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG]: Reading gsuite_group_members")
	//var diags diag.Diagnostics

	if (d.Get("group_id")) != "" {
		groupId := d.Get("group_id").(string)
		d.SetId(fmt.Sprintf("groups/%s", groupId))
	} else {

		client := meta.(*apiClient)

		directoryService, diags := client.NewDirectoryService()
		if diags.HasError() {
			return diags
		}

		membersService, diags := GetMembersService(directoryService)
		if diags.HasError() {
			return diags
		}

		groupId := d.Get("group_id").(string)

		members, err := membersService.List(groupId).Do()
		if err != nil {
			return handleNotFoundError(err, d, d.Id())
		}

		if members == nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("No group members were returned for group %s", groupId),
			})

			return diags
		}

		//d.Set("group_id", "val")

		//groupEmail := d.Id()

		//d.Set("group_email", strings.ToLower(groupEmail))
	}

	return resourceGroupMembersRead(ctx, d, meta)
}

//func membersToCfg(members *Members) []map[string]interface{} {
//	if members == nil {
//		return nil
//	}
//
//	finalMembers := make([]map[string]interface{}, 0, len(members))
//
//	for _, m := range members {
//		finalMembers = append(finalMembers, map[string]interface{}{
//			"email":  m.Email,
//			"etag":   m.Etag,
//			"kind":   m.Kind,
//			"status": m.Status,
//			"type":   m.Type,
//			"role":   m.Role,
//		})
//	}
//
//	return finalMembers
//}

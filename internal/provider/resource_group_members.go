package googleworkspace

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	directory "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	"log"
	"strings"
	"time"
)

func resourceGroupMembers() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Group resource manages Google Workspace Groups.",

		CreateContext: resourceGroupMembersCreate,
		ReadContext:   resourceGroupMembersRead,
		UpdateContext: resourceGroupMembersUpdate,
		DeleteContext: resourceGroupMembersDelete,

		Importer: &schema.ResourceImporter{
			StateContext: resourceGroupMembersImporter,
		},

		Schema: map[string]*schema.Schema{
			"group_id": {
				Description: "Identifies the group in the API request. The value can be the group's email address, " +
					"group alias, or the unique group ID.",
				Type:     schema.TypeString,
				Required: true,
			},
			"member": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"email": {
							Description: "The member's email address. A member can be a user or another group. This property is" +
								"required when adding a member to a group. The email must be unique and cannot be an alias of" +
								"another group. If the email address is changed, the API automatically reflects the email address changes.",
							Type:     schema.TypeString,
							Required: true,
						},
						"etag": {
							Description: "ETag of the resource.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"role": {
							Description: "The member's role in a group. The API returns an error for cycles in group memberships. " +
								"For example, if group1 is a member of group2, group2 cannot be a member of group1. " +
								"Acceptable values are: " +
								"`MANAGER`: This role is only available if the Google Groups for Business is " +
								"enabled using the Admin Console. A `MANAGER` role can do everything done by an `OWNER` role except " +
								"make a member an `OWNER` or delete the group. A group can have multiple `MANAGER` members. " +
								"`MEMBER`: This role can subscribe to a group, view discussion archives, and view the group's " +
								"membership list. " +
								"`OWNER`: This role can send messages to the group, add or remove members, change member roles, " +
								"change group's settings, and delete the group. An OWNER must be a member of the group. " +
								"A group can have more than one OWNER.",
							Type:     schema.TypeString,
							Optional: true,
							Default:  "MEMBER",
							ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"MANAGER", "MEMBER", "OWNER"},
								false)),
						},
					},
				},
			},
			// Adding a computed id simply to override the `optional` id that gets added in the SDK
			// that will then display improperly in the docs
			"id": {
				Description: "The ID of this resource.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceGroupMembersRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	/*
		log.Printf("[DEBUG]: Reading gsuite_group_members")
			config := meta.(*Config)

			groupEmail := d.Id()

			members, err := getAPIMembers(groupEmail, config)

			if err != nil {
				return err
			}

			d.Set("group_email", strings.ToLower(groupEmail))
			d.Set("member", membersToCfg(members))
			return nil

	*/

	var diags diag.Diagnostics

	// use the meta value to retrieve your client from the provider configure method
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
	//memberId := d.Get("member_id").(string)

	members, err := membersService.List(groupId).Do()
	if err != nil {
		return handleNotFoundError(err, d, d.Id())
	}

	if members == nil {
		return nil
	}

	memberList, diags := getMembers(groupId, membersService)
	if diags.HasError() {
		log.Printf("[DEBUG]: Issue calling getMembers")
	}

	finalMembers := make([]map[string]interface{}, 0, len(memberList))

	for _, m := range memberList {
		finalMembers = append(finalMembers, map[string]interface{}{
			//"group_id": groupId,
			"email": m.Email,
			"etag":  m.Etag,
			//"kind":     m.Kind,
			//"status":   m.Status,
			//"type":     m.Type,
			"role": m.Role,
		})
	}

	d.SetId(fmt.Sprintf("groups/%s", groupId))
	d.Set("member", finalMembers)

	return diags
}

func resourceGroupMembersCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	log.Printf("[DEBUG]: Creating gsuite_group_members")
	gid, err := createOrUpdateGroupMembers(ctx, d, meta)

	if err != nil {
		return err
	}

	d.SetId(gid)
	return resourceGroupMembersRead(ctx, d, meta)
}

func resourceGroupMembersUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG]: Updating gsuite_group_members")
	_, err := createOrUpdateGroupMembers(ctx, d, meta)

	if err != nil {
		return err
	}
	return resourceGroupMembersRead(ctx, d, meta)
}

func resourceGroupMembersDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[DEBUG]: Deleting gsuite_group_members")

	for _, rawMember := range d.Get("member").(*schema.Set).List() {
		member := rawMember.(map[string]interface{})
		deleteMember(ctx, d.Timeout(schema.TimeoutDelete), member["email"].(string), d.Id(), meta)
	}

	d.SetId("")
	return nil
}

func membersToCfg(members []*directory.Member) []map[string]interface{} {
	if members == nil {
		return nil
	}

	finalMembers := make([]map[string]interface{}, 0, len(members))

	for _, m := range members {
		finalMembers = append(finalMembers, map[string]interface{}{
			"email":  m.Email,
			"etag":   m.Etag,
			"kind":   m.Kind,
			"status": m.Status,
			"type":   m.Type,
			"role":   m.Role,
		})
	}

	return finalMembers
}

func resourceMembers(d *schema.ResourceData) (members []map[string]interface{}) {
	for _, rawMember := range d.Get("member").(*schema.Set).List() {
		member := rawMember.(map[string]interface{})
		members = append(members, member)
	}
	return members
}

func createOrUpdateGroupMembers(ctx context.Context, d *schema.ResourceData, meta interface{}) (string, diag.Diagnostics) {
	var test = d.Get("group_id")
	print(test)
	groupEmail := strings.ToLower(d.Get("group_id").(string))

	// Get members from config
	cfgMembers := resourceMembers(d)

	// Get members from API
	apiMembers, err := getAPIMembers(ctx, groupEmail, d.Timeout(schema.TimeoutUpdate), meta)
	if err != nil {
		return groupEmail, diag.Errorf("[ERROR] Error updating memberships: %v", err)
	}
	// This call removes any members that aren't defined in cfgMembers,
	// and adds all of those that are
	err = reconcileMembers(ctx, d, cfgMembers, membersToCfg(apiMembers), groupEmail, meta)
	if err != nil {
		return groupEmail, diag.Errorf("[ERROR] Error updating memberships: %v", err)
	}

	return groupEmail, nil
}

func getMembers(groupEmail string, service *directory.MembersService) ([]*directory.Member, diag.Diagnostics) {

	groupMembers := make([]*directory.Member, 0)
	token := ""
	var membersResponse *directory.Members
	var err error
	for paginate := true; paginate; {

		membersResponse, err = service.List(groupEmail).PageToken(token).Do()

		if err != nil {
			return groupMembers, diag.FromErr(err)
		}
		for _, v := range membersResponse.Members {
			groupMembers = append(groupMembers, v)
		}
		token = membersResponse.NextPageToken
		paginate = token != ""
	}
	return groupMembers, diag.FromErr(err)
}

// This function ensures that the members of a group exactly match that
// in a config by deleting any members that are returned by the API but not present
// in the config
func reconcileMembers(ctx context.Context, d *schema.ResourceData, cfgMembers, apiMembers []map[string]interface{}, gid string, meta interface{}) diag.Diagnostics {

	// Helper to convert slice to map
	m := func(vals []map[string]interface{}) map[string]map[string]interface{} {
		sm := make(map[string]map[string]interface{})
		for _, member := range vals {
			email := strings.ToLower(member["email"].(string))
			member["email"] = strings.ToLower(member["email"].(string))
			sm[email] = member
		}
		return sm
	}

	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return diags
	}

	membersService, diags := GetMembersService(directoryService)
	if diags.HasError() {
		return diags
	}

	cfgMap := m(cfgMembers)
	log.Println("[DEBUG] Members in cfg: ", cfgMap)
	apiMap := m(apiMembers)
	log.Println("[DEBUG] Member in API: ", apiMap)

	var cfgRole, apiRole string

	for k, apiMember := range apiMap {
		if cfgMember, ok := cfgMap[k]; !ok {
			// The member in the API is not in the config; disable it.
			log.Printf("[DEBUG] Member in API not in config. Disabling it: %s", k)
			err := deleteMember(ctx, d.Timeout(schema.TimeoutDelete), k, gid, meta)
			if err != nil {
				return err
			}
		} else {
			// The member exists in the config and the API
			// If role has changed update, otherwise do nothing
			cfgRole = strings.ToUpper(cfgMember["role"].(string))
			apiRole = strings.ToUpper(apiMember["role"].(string))
			if cfgRole != apiRole {
				groupMember := &directory.Member{
					Role: cfgRole,
				}

				var updatedGroupMember *directory.Member
				var err error
				err = retryNotFound(ctx, func() error {
					updatedGroupMember, err = membersService.Patch(
						strings.ToLower(d.Get("group_id").(string)),
						strings.ToLower(cfgMember["email"].(string)),
						groupMember).Do()
					return err
				}, d.Timeout(schema.TimeoutUpdate))

				if err != nil {
					return diag.FromErr(err)
				}
				//if err != nil {
				//	return fmt.Errorf("[ERROR] Error updating groupMember: %s", err)
				//}

				log.Printf("[INFO] Updated groupMember: %s", updatedGroupMember.Email)
			}

			// Delete from cfgMap, we have already handled it
			delete(cfgMap, k)
		}
	}

	// Upsert memberships which are present in the config, but not in the api
	for email := range cfgMap {
		err := upsertMember(ctx, d.Timeout(schema.TimeoutUpdate), email, gid, cfgMap[email]["role"].(string), meta)
		if err != nil {
			return err
		}
	}
	return nil
}

// Retrieve a group's members from the API
func getAPIMembers(ctx context.Context, groupEmail string, timeoutDuration time.Duration, meta interface{}) ([]*directory.Member, diag.Diagnostics) {
	// Get members from the API
	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return nil, diags
	}

	membersService, diags := GetMembersService(directoryService)
	if diags.HasError() {
		return nil, diags
	}

	groupMembers := make([]*directory.Member, 0)
	token := ""
	var membersResponse *directory.Members
	var err error
	for paginate := true; paginate; {

		err = retry(ctx, func() error {
			membersResponse, err = membersService.List(groupEmail).PageToken(token).Do()
			return err
		}, timeoutDuration)

		if err != nil {
			return groupMembers, diag.FromErr(err)
		}

		for _, v := range membersResponse.Members {
			groupMembers = append(groupMembers, v)
		}
		token = membersResponse.NextPageToken
		paginate = token != ""
	}
	return groupMembers, nil
}

func upsertMember(ctx context.Context, timeoutDuration time.Duration, email, groupEmail, role string, meta interface{}) diag.Diagnostics {
	var err error

	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return diags
	}

	membersService, diags := GetMembersService(directoryService)
	if diags.HasError() {
		return diags
	}

	groupMember := &directory.Member{
		Role:  strings.ToUpper(role),
		Email: strings.ToLower(email),
	}

	// Check if the email address belongs to a user, or to a group
	// we need to make sure, because we need to use different logic
	var isGroup = true
	err = retry(ctx, func() error {
		_, err := membersService.Get(groupEmail, email).Do()
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 404 {
			isGroup = false
			log.Printf("[DEBUG] Setting isGroup to false for %s after getting a 404", email)
			return nil
		}
		return err
	}, timeoutDuration)

	if isGroup == true {
		if role != "MEMBER" {
			return diag.Errorf("[ERROR] Error creating groupMember (%s): nested groups should be role MEMBER", email)
			//return fmt.Errorf()
		}

		var isGroupMember = true

		// Grab the group as a directory member of the current group
		err = retry(ctx, func() error {
			_, err := membersService.Get(groupEmail, email).Do()

			if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 404 {
				isGroupMember = false
				log.Printf("[DEBUG] Setting isGroupMember to false for %s after getting a 404", email)
				return nil
			}

			return err
		}, timeoutDuration)

		// Based on the err return, either add as a new member, or update
		if isGroupMember == false {
			var createdGroupMember *directory.Member
			err = retry(ctx, func() error {
				createdGroupMember, err = membersService.Insert(groupEmail, groupMember).Do()
				return err
			}, timeoutDuration)
			if err != nil {
				return diag.Errorf("[ERROR] Error creating groupMember: %s, %s", err, email)
			}
			log.Printf("[INFO] Created groupMember: %s", createdGroupMember.Email)
		} else {
			var updatedGroupMember *directory.Member
			err = retryNotFound(ctx, func() error {
				updatedGroupMember, err = membersService.Update(groupEmail, email, groupMember).Do()
				return err
			}, timeoutDuration)
			if err != nil {
				return diag.Errorf("[ERROR] Error updating groupMember: %s, %s", err, email)
			}
			log.Printf("[INFO] Updated groupMember: %s", updatedGroupMember.Email)
		}
	}

	if isGroup == false {
		// Basically the same check as group, but using a more apt method "HasMember"
		// specifically meant for users
		var hasMemberResponse *directory.MembersHasMember
		var err error
		err = retry(ctx, func() error {
			hasMemberResponse, err = membersService.HasMember(groupEmail, email).Do()
			if err == nil {
				return nil
			}

			// When a user does not exist, the API returns a 400 "memberKey, required"
			// Returning a friendly message
			if gerr, ok := err.(*googleapi.Error); ok && (gerr.Errors[0].Reason == "required" && gerr.Code == 400) {
				return fmt.Errorf("[ERROR] Error adding groupMember %s, please make sure the user exists beforehand", email)
			}
			return err
		}, timeoutDuration)
		if err != nil {
			return createGroupMember(ctx, timeoutDuration, groupMember, groupEmail, meta)
		}

		if hasMemberResponse.IsMember == true {
			var updatedGroupMember *directory.Member
			err = retryNotFound(ctx, func() error {
				updatedGroupMember, err = membersService.Update(groupEmail, email, groupMember).Do()
				return err
			}, timeoutDuration)
			if err != nil {
				return diag.Errorf("[ERROR] Error updating groupMember: %s, %s", err, email)
			}
			log.Printf("[INFO] Updated groupMember: %s", updatedGroupMember.Email)
		} else {
			return createGroupMember(ctx, timeoutDuration, groupMember, groupEmail, meta)
		}
	}

	return nil
}

func createGroupMember(ctx context.Context, timeoutDuration time.Duration, groupMember *directory.Member, groupEmail string, meta interface{}) diag.Diagnostics {

	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return diags
	}

	membersService, diags := GetMembersService(directoryService)
	if diags.HasError() {
		return diags
	}

	var err error
	var createdGroupMember *directory.Member
	err = retry(ctx, func() error {
		createdGroupMember, err = membersService.Insert(groupEmail, groupMember).Do()
		return err
	}, timeoutDuration)
	if err != nil {
		return diag.Errorf("[ERROR] Error creating groupMember: %s, %s", err, groupMember.Email)
	}
	log.Printf("[INFO] Created groupMember: %s", createdGroupMember.Email)

	return nil
}

func deleteMember(ctx context.Context, timeoutDuration time.Duration, email, groupEmail string, meta interface{}) diag.Diagnostics {

	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return diags
	}

	membersService, diags := GetMembersService(directoryService)
	if diags.HasError() {
		return diags
	}

	var err error
	err = retry(ctx, func() error {
		err = membersService.Delete(groupEmail, email).Do()
		return err
	}, timeoutDuration)

	if err != nil {
		return diag.Errorf("[ERROR] Error deleting member: %s", err)
	}
	return nil
}

// Allow importing using any groupKey (id, email, alias)
func resourceGroupMembersImporter(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	log.Printf("[DEBUG] Importing gsuite_group_members")
	//config := meta.(*Config)
	client := meta.(*apiClient)

	directoryService, diags := client.NewDirectoryService()
	if diags.HasError() {
		return nil, diags[len(diags)-1].Validate()
	}

	var group *directory.Group
	var members []*directory.Member
	var err error
	err = retry(ctx, func() error {
		group, err = directoryService.Groups.Get(strings.ToLower(d.Id())).Do()
		return err
	}, d.Timeout(schema.TimeoutCreate))

	if err != nil {
		return nil, fmt.Errorf("[ERROR] Error fetching group. Make sure the group exists: %s ", err)
	}

	d.Set("group_id", group.Id)
	d.SetId(fmt.Sprintf("groups/%s", group.Id))

	var diagz diag.Diagnostics
	err = retry(ctx, func() error {
		members, diagz = getAPIMembers(ctx, group.Email, d.Timeout(schema.TimeoutRead), meta)
		if diagz.HasError() {
			return diagz[len(diagz)-1].Validate()
		}
		return nil
	}, d.Timeout(schema.TimeoutRead))

	if err != nil {
		return nil, fmt.Errorf("[ERROR] Error fetching group members. Make sure the group exists: %s ", err)
	}

	finalMembers := make([]map[string]interface{}, 0, len(members))

	for _, m := range members {
		finalMembers = append(finalMembers, map[string]interface{}{
			//"group_id": groupId,
			"email": m.Email,
			"etag":  m.Etag,
			//"kind":     m.Kind,
			//"status":   m.Status,
			//"type":     m.Type,
			"role": m.Role,
		})
	}

	d.Set("member", finalMembers)

	return []*schema.ResourceData{d}, nil
}

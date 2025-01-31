---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "googleworkspace_group_member Data Source - terraform-provider-googleworkspace"
subcategory: ""
description: |-
  Group Member data source in the Terraform Googleworkspace provider.
---

# googleworkspace_group_member (Data Source)

Group Member data source in the Terraform Googleworkspace provider.

## Example Usage

```terraform
data "googleworkspace_group" "sales" {
  email = "sales@example.com"
}

data "googleworkspace_group_member" "my-group-member" {
  group_id = data.googleworkspace_group.sales.id
  email    = "michael.scott@example.com"
}

output "group_member_role" {
  value = data.googleworkspace_group_member.my-group-member.role
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- **group_id** (String) Identifies the group in the API request. The value can be the group's email address, group alias, or the unique group ID.

### Optional

- **email** (String) The member's email address. A member can be a user or another group. This property isrequired when adding a member to a group. The email must be unique and cannot be an alias ofanother group. If the email address is changed, the API automatically reflects the email address changes.
- **member_id** (String) The unique ID of the group member. A member id can be used as a member request URI's memberKey.

### Read-Only

- **delivery_settings** (String) Defines mail delivery preferences of member. Acceptable values are:`ALL_MAIL`: All messages, delivered as soon as they arrive. `DAILY`: No more than one message a day. `DIGEST`: Up to 25 messages bundled into a single message. `DISABLED`: Remove subscription. `NONE`: No messages.
- **etag** (String) ETag of the resource.
- **id** (String) The ID of this resource.
- **role** (String) The member's role in a group. The API returns an error for cycles in group memberships. For example, if group1 is a member of group2, group2 cannot be a member of group1. Acceptable values are: `MANAGER`: This role is only available if the Google Groups for Business is enabled using the Admin Console. A `MANAGER` role can do everything done by an `OWNER` role except make a member an `OWNER` or delete the group. A group can have multiple `MANAGER` members. `MEMBER`: This role can subscribe to a group, view discussion archives, and view the group's membership list. `OWNER`: This role can send messages to the group, add or remove members, change member roles, change group's settings, and delete the group. An OWNER must be a member of the group. A group can have more than one OWNER.
- **status** (String) Status of member.
- **type** (String) The type of group member. Acceptable values are: `CUSTOMER`: The member represents all users in a domain. An email address is not returned and the ID returned is the customer ID. `GROUP`: The member is another group. `USER`: The member is a user.



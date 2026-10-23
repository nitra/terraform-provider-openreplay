---
page_title: "openreplay_project Resource - OpenReplay"
description: |-
  Manage an OpenReplay project and retrieve its tracker key.
---

# openreplay_project (Resource)

Creates, reads, updates, and deletes an OpenReplay project through the authenticated dashboard API. The provider requires an owner/admin dashboard JWT.

## Example usage

```hcl
resource "openreplay_project" "web" {
  name     = "Production"
  platform = "web"
}

output "project_key" {
  value = openreplay_project.web.project_key
}
```

## Schema

| Attribute | Type | Description |
| --- | --- | --- |
| `name` | String, required | Project name. |
| `platform` | String, required | Project platform, either `web` or `ios`. |
| `id` | Number, computed | Numeric dashboard project ID. |
| `project_key` | String, computed | Project key used to configure the OpenReplay tracker. |

Both `name` and `platform` support updates. The resource manages these fields and reads its ID and tracker key; other server settings are outside its schema.

If refresh confirms the project is missing, the resource is removed from state and a later plan can recreate it. Other API errors produce diagnostics. Destroy sends a deletion request before removing the resource from state.

## Import

Declare the resource in your configuration and import it using its numeric dashboard project ID:

```sh
tofu import openreplay_project.web 42
```

The import ID must be a decimal integer within the signed 64-bit range. The provider reads the project to populate `name`, `platform`, and `project_key`. A tracker key cannot be used as the import ID. Configure provider authentication before running import.

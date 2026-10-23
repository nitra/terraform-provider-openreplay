---
page_title: "OpenReplay Provider"
description: |-
  Manage self-hosted OpenReplay projects through the authenticated dashboard API.
---

# OpenReplay Provider

Use provider source `nitra/openreplay` to manage projects through the OpenReplay dashboard API. Authentication requires a dashboard JWT belonging to an owner or admin. The organization API-key workflow and release automation are deferred; see the [README](../README.md) for local installation.

## Example usage

```hcl
terraform {
  required_providers {
    openreplay = {
      source = "nitra/openreplay"
    }
  }
}

variable "openreplay_token" {
  type      = string
  sensitive = true
}

provider "openreplay" {
  endpoint = "https://replay.example.com/api"
  token    = var.openreplay_token
}
```

## Configuration

| Argument | Type | Description |
| --- | --- | --- |
| `endpoint` | String, optional | Absolute HTTP(S) dashboard URL. Defaults to `OPENREPLAY_ENDPOINT` when omitted. |
| `token` | String, optional, sensitive | Owner/admin dashboard JWT. Defaults to `OPENREPLAY_TOKEN` when omitted. |

Both settings are required after environment fallback. Explicit HCL values take precedence, including an explicit empty value, which is rejected. Values must be known before provider configuration.

The endpoint path may be empty, `/`, `/api`, or `/api/`; requests use `/api` as their base path. URLs containing credentials, query strings, fragments, or other paths are rejected. The token must be nonempty and contain no whitespace. Requests send it as an `Authorization: Bearer` header. Provider configuration is never copied into project resource state.

For environment configuration, use an empty provider block:

```hcl
provider "openreplay" {}
```

```sh
export OPENREPLAY_ENDPOINT='https://replay.example.com/api'
export OPENREPLAY_TOKEN='<owner-or-admin-dashboard-jwt>'
```

## Resources

- [`openreplay_project`](resources/project.md): manage an OpenReplay project and retrieve its tracker key.

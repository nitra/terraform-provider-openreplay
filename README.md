# OpenReplay provider for OpenTofu

Manage self-hosted OpenReplay projects with the `nitra/openreplay` provider. The `openreplay_project` resource creates and updates projects through the dashboard API and returns the `project_key` used by the OpenReplay tracker.

## Configure a project

Authenticate with an owner/admin dashboard JWT. Set the token through a sensitive input variable or `OPENREPLAY_TOKEN`.

```hcl
terraform {
  required_providers {
    openreplay = {
      source = "nitra/openreplay"
    }
  }
}

variable "openreplay_token" {
  type        = string
  sensitive   = true
  description = "Owner/admin OpenReplay dashboard JWT."
}

provider "openreplay" {
  endpoint = "https://replay.example.com/api"
  token    = var.openreplay_token
}

resource "openreplay_project" "web" {
  name     = "Production"
  platform = "web"
}

output "project_key" {
  value = openreplay_project.web.project_key
}
```

For this example, supply the JWT using `TF_VAR_openreplay_token`. Keep credentials out of version control.

Alternatively, omit `endpoint` and `token` from the provider block and set:

```sh
export OPENREPLAY_ENDPOINT='https://replay.example.com/api'
export OPENREPLAY_TOKEN='<owner-or-admin-dashboard-jwt>'
```

Explicit provider arguments take precedence over environment variables. Both values must be available when the provider is configured. The token is sensitive provider configuration and is never copied into project resource state.

The endpoint accepts an absolute HTTP(S) URL with an empty path or `/api`; the client normalizes it to `/api`. Credentials, query strings, fragments, and other paths are rejected.

## Import an existing project

Add the matching resource configuration, then import using the numeric dashboard project ID:

```sh
tofu import openreplay_project.web 42
```

The next refresh reads the project's name, platform, and tracker key. Use the numeric ID rather than the tracker key as the import argument.

See the [provider reference](docs/index.md), [project reference](docs/resources/project.md), and [examples](examples).

## Build and use locally

Registry publication and release automation are deferred. To use this checkout, install Go 1.25 or later and OpenTofu with protocol v6 support, then build:

```sh
go build -o bin/terraform-provider-openreplay .
```

Add a development override to your OpenTofu CLI configuration (for example, `~/.tofurc`). Replace the path below with the absolute path to this checkout's `bin` directory:

```hcl
provider_installation {
  dev_overrides {
    "registry.opentofu.org/nitra/openreplay" = "/absolute/path/to/terraform-provider-openreplay/bin"
  }
  direct {}
}
```

Put the project configuration in a separate directory. With this development override active, run `tofu plan` or `tofu apply` directly. `tofu init` still attempts registry installation, so skip it for a configuration using only this local provider. Remove the override when switching to a published release.

Run the Go tests with:

```sh
go test ./...
```

Tests use a local HTTP test server; live OpenReplay acceptance tests are deferred.

## Scope

The provider manages project `name` and `platform` (`web` or `ios`) and reads the numeric `id` and `project_key`. Destroying the resource sends a project deletion request to OpenReplay.

The organization API-key workflow is deferred. This version requires an owner/admin dashboard JWT for the project create, read, update, and delete operations. Registry publishing, signing, and release automation are also deferred.

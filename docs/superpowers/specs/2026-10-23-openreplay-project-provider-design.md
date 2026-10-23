# OpenReplay Project Provider Design

## Intent

Create the public `nitra/terraform-provider-openreplay` repository. Its first release lets an OpenTofu configuration manage OpenReplay projects through the self-hosted dashboard API. The provider removes the manual project-creation step and returns the project key needed by application trackers.

## Goals

- Publish a Go provider whose source address is `nitra/openreplay` and whose local name is `openreplay`.
- Provide `openreplay_project` with full create, read, update, delete, and import lifecycle support.
- Use the authenticated dashboard API endpoints: `POST /projects`, `GET /projects/{id}`, `PUT /projects/{id}`, and `DELETE /projects/{id}`, all relative to an API endpoint such as `https://replay.example.com/api`.
- Support a `name` and `platform` (`web` or `ios`) on projects; expose numeric `id` and `project_key` from the server.
- Obtain provider configuration from HCL or `OPENREPLAY_ENDPOINT` and `OPENREPLAY_TOKEN`; mark the token sensitive and never persist it in resource state.
- Test the provider and client against `httptest` rather than requiring a running OpenReplay instance.
- Include README, generated-style provider/resource documentation, and executable-looking HCL examples.

## Non-goals

- Manage OpenReplay users, organization settings, alerts, integrations, GDPR settings, or session data.
- Support the documented organization API key workflow in the first release because its confirmed API surface does not provide the update and delete operations required for a normal managed resource.
- Create registry publishing, signing, or GitHub Actions release automation in the first release.

## Architecture

The provider uses `terraform-plugin-framework` and protocol v6. `main.go` serves a provider factory in an internal `provider` package. The provider schema owns endpoint and JWT configuration. It builds a small standard-library HTTP client that appends dashboard paths to the endpoint, sends `Authorization: Bearer <token>`, decodes OpenReplay's `{ "data": ... }` envelope, and turns non-success responses into actionable diagnostics.

The client sends `{ "name": string, "platform": "web" | "ios" }` to create and update. Create and read return `data.projectId`, `data.projectKey`, `data.name`, `data.platform`, and may return `data.saveRequestPayloads` plus other server-owned fields; the resource consumes the first four and ignores extras. Delete returns `{ "data": { "state": "success" } }`. A duplicate name returns HTTP 400 with FastAPI's `{ "detail": string }`; authorization failures may return a normal success status with `{ "errors": ["unauthorized"] }`, which the client must treat as an error. A missing project from Read returns `{ "errors": ["project not found"] }`; the HTTP client represents it as absence even if the server does not use 404.

`openreplay_project` has a model shared by its Terraform schema and the HTTP client mapping. Create stores the numeric project ID from the API response. Read refreshes all computed values and removes the resource from state only for a confirmed 404. Update sends both mutable fields. Delete sends the API request then clears state. Import accepts the numeric project ID and relies on Read to populate state.

## Resource contract

```hcl
terraform {
  required_providers {
    openreplay = {
      source = "nitra/openreplay"
    }
  }
}

provider "openreplay" {
  endpoint = "https://replay.example.com/api"
  token    = var.openreplay_jwt
}

resource "openreplay_project" "production" {
  name     = "Production"
  platform = "web"
}
```

`endpoint` must be an absolute HTTP(S) URL. `token` is required after HCL/environment resolution. `name` and `platform` are required. `platform` accepts only `web` and `ios`. The dashboard's numeric `project_id` is the Terraform ID. `project_key` is computed and can be passed to the OpenReplay tracker.

## Authentication and failure behavior

The provider needs a dashboard JWT that belongs to an OpenReplay owner or admin. It sets it in a bearer Authorization header. A 401 or 403 should report the request method/path and explain that a valid owner/admin dashboard JWT is needed, without including credentials. A non-404 API error leaves the existing state intact. API response validation errors include the returned status and safe server message.

## Verification

Unit tests exercise endpoint validation, environment fallback, sensitive token schema, request method/path/body/header behavior, decoding, lifecycle responses, 404 disappearance, and 401/403 diagnostics. The full Go test suite must pass before release. A live acceptance suite and release pipeline are deferred.

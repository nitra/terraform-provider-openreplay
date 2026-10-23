# OpenReplay Project Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first `nitra/openreplay` OpenTofu provider release, which manages OpenReplay dashboard projects with full lifecycle support.

**Architecture:** A Terraform Plugin Framework provider configures a narrow HTTP client through a dashboard API endpoint and owner/admin JWT. A single project resource maps the API contract into Terraform state, while client and resource unit tests use `httptest` to verify all requests and lifecycle behavior without a live server.

**Tech Stack:** Go, `terraform-plugin-framework`, `terraform-plugin-framework-validators`, Go `net/http`, `httptest`, OpenTofu protocol v6.

**Spec:** `docs/superpowers/specs/2026-10-23-openreplay-project-provider-design.md`

## Global Constraints

- The provider source address is `nitra/openreplay`; its Terraform type prefix is `openreplay`.
- Use `terraform-plugin-framework` and protocol v6; do not use SDKv2.
- Provider configuration accepts HCL and `OPENREPLAY_ENDPOINT` / `OPENREPLAY_TOKEN` environment fallbacks.
- Only absolute `http` and `https` endpoint URLs are valid; credentials, query, fragment, and a path other than `/api` are invalid.
- `openreplay_project` manages only `name` and `platform`; platform values are exactly `web` and `ios`.
- The provider JWT is sensitive and must never become resource state, a diagnostic, or a log entry.
- Treat `errors` envelopes as errors even when OpenReplay answers with a 2xx status.
- For `GET /projects/{id}`, an HTTP 404 or `errors: ["project not found"]` removes the resource from state.

## Review Focus

- A trailing slash in endpoint input yields exactly one `/api/projects` path; test this in Task 1.
- JWT credentials in an endpoint URL are rejected and never copied into diagnostics; test this in Task 1.
- An API `200` with `errors: ["unauthorized"]` fails resource operations; test this in Task 2.
- A project that vanishes through the documented JSON absence envelope clears state during Read; test this in Task 3.
- Import with a non-numeric ID reports a validation diagnostic before issuing an HTTP request; test this in Task 3.

### Task 1: Provider configuration and HTTP client

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/openreplay/client.go`
- Create: `internal/openreplay/client_test.go`
- Create: `internal/provider/provider.go`
- Create: `internal/provider/provider_test.go`

**Interfaces:**
- Produces: `openreplay.NewClient(endpoint, token string) (*Client, error)` and `provider.New(version string) func() provider.Provider`.
- Produces: `Client.Do(ctx context.Context, method, path string, body any, output any) error` for resource code.

- [ ] **Step 1: Initialize the Go module and add Plugin Framework dependencies**

Set module name to `github.com/nitra/terraform-provider-openreplay`; add current compatible framework and validator dependencies.

- [ ] **Step 2: Write failing client tests for endpoint normalization and rejection**

```go
func TestNewClientNormalizesAPITrailingSlash(t *testing.T) {
    client, err := NewClient(server.URL+"/api/", "jwt")
    require.NoError(t, err)
    require.Equal(t, "/api/projects", client.URL("/projects").Path)
}

func TestNewClientRejectsEndpointCredentials(t *testing.T) {
    _, err := NewClient("https://secret@example.test/api", "jwt")
    require.Error(t, err)
}
```

- [ ] **Step 3: Run client tests to verify they fail**

Run: `go test ./internal/openreplay -run 'TestNewClient' -v`

Expected: FAIL because `NewClient` does not exist.

- [ ] **Step 4: Implement `NewClient(endpoint, token string) (*Client, error)` and `Client.Do` in `internal/openreplay/client.go`**

Use `net/url` to validate and normalize the base endpoint. Use `net/http` with a context-aware request, bearer token, JSON request encoding, JSON envelope decoding, and safe error messages.

- [ ] **Step 5: Write failing provider tests for environment fallback and sensitive token schema**

```go
func TestProviderSchemaMarksTokenSensitive(t *testing.T) {
    schema := providerSchema(t)
    require.True(t, schema.Attributes["token"].(schema.StringAttribute).Sensitive)
}
```

- [ ] **Step 6: Run provider tests to verify they fail**

Run: `go test ./internal/provider -run 'TestProviderSchema' -v`

Expected: FAIL because the provider schema is absent.

- [ ] **Step 7: Implement provider metadata, schema, and configuration in `internal/provider/provider.go`**

Use `DefaultFunc` environment fallbacks, `Sensitive: true` for the token, diagnostics for missing/invalid configuration, and configure the client as resource provider data.

- [ ] **Step 8: Run targeted tests and the full suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum main.go internal/openreplay internal/provider
git commit -m "feat: add OpenReplay provider configuration"
```

### Task 2: Project API contract

**Files:**
- Modify: `internal/openreplay/client.go`
- Create: `internal/openreplay/projects.go`
- Create: `internal/openreplay/projects_test.go`

**Interfaces:**
- Consumes: `Client.Do(ctx, method, path, body, output)` from Task 1.
- Produces: `Client.CreateProject(ctx, ProjectInput) (Project, error)`, `GetProject(ctx, int64) (Project, error)`, `UpdateProject(ctx, int64, ProjectInput) (Project, error)`, and `DeleteProject(ctx, int64) error`.
- Produces: `ProjectInput{Name string; Platform string}` and `Project{ID int64; Key, Name, Platform string}`.

- [ ] **Step 1: Write failing HTTP contract tests for project CRUD**

```go
func TestCreateProjectSendsDashboardContract(t *testing.T) {
    // Assert POST /api/projects, bearer authorization, and {"name":"Production","platform":"web"}.
    project, err := client.CreateProject(ctx, ProjectInput{Name: "Production", Platform: "web"})
    require.NoError(t, err)
    require.Equal(t, int64(42), project.ID)
    require.Equal(t, "project-key", project.Key)
}
```

- [ ] **Step 2: Run contract tests to verify they fail**

Run: `go test ./internal/openreplay -run 'Test(Create|Get|Update|Delete)Project' -v`

Expected: FAIL because project client methods do not exist.

- [ ] **Step 3: Implement project request/response models and CRUD methods in `internal/openreplay/projects.go`**

Map JSON `projectId` and `projectKey`; ignore extra server fields. Map HTTP 404 and a `project not found` errors envelope to a typed `ErrNotFound`; map authorization envelopes and HTTP 401/403 to a typed authorization error.

- [ ] **Step 4: Add failing tests for API error envelopes**

```go
func TestGetProjectTreatsUnauthorizedEnvelopeAsError(t *testing.T) {
    // Server returns 200 and {"errors":["unauthorized"]}.
    _, err := client.GetProject(ctx, 42)
    require.ErrorIs(t, err, ErrUnauthorized)
}
```

- [ ] **Step 5: Run error tests to verify they fail, then implement the minimal envelope classification**

Run: `go test ./internal/openreplay -run 'Test.*(Unauthorized|NotFound)' -v`

Expected before implementation: FAIL; expected after implementation: PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/openreplay
git commit -m "feat: add OpenReplay project API client"
```

### Task 3: `openreplay_project` resource

**Files:**
- Create: `internal/provider/project_resource.go`
- Create: `internal/provider/project_resource_test.go`
- Modify: `internal/provider/provider.go`

**Interfaces:**
- Consumes: `openreplay.Client` project CRUD methods from Task 2.
- Produces: a Framework `resource.Resource` registered as `openreplay_project`.

- [ ] **Step 1: Write failing resource schema tests**

```go
func TestProjectResourceSchema(t *testing.T) {
    schema := resourceSchema(t)
    require.True(t, schema.Attributes["name"].(schema.StringAttribute).Required)
    require.True(t, schema.Attributes["platform"].(schema.StringAttribute).Required)
    require.True(t, schema.Attributes["project_key"].(schema.StringAttribute).Computed)
}
```

- [ ] **Step 2: Run resource schema test to verify it fails**

Run: `go test ./internal/provider -run TestProjectResourceSchema -v`

Expected: FAIL because the resource is absent.

- [ ] **Step 3: Implement the project resource schema and registration**

Use an `Int64` computed `id`, `String` computed `project_key`, required `name`, and required `platform` with exactly `web`/`ios` validation.

- [ ] **Step 4: Write failing lifecycle tests with a test client**

```go
func TestProjectReadRemovesStateForMissingProject(t *testing.T) {
    // A typed ErrNotFound must produce an empty Terraform state.
}

func TestProjectImportRejectsNonNumericID(t *testing.T) {
    // Assert validation error and no client request.
}
```

- [ ] **Step 5: Run lifecycle tests to verify they fail**

Run: `go test ./internal/provider -run 'TestProject(Read|Import)' -v`

Expected: FAIL because lifecycle behavior is absent.

- [ ] **Step 6: Implement Create, Read, Update, Delete, ImportState, and Configure**

Pass all diagnostics through Framework response objects. Delete clears state after successful deletion; Read preserves state for all failures other than typed not-found.

- [ ] **Step 7: Run targeted and full tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/provider
git commit -m "feat: manage OpenReplay projects"
```

### Task 4: Documentation and repository usability

**Files:**
- Create: `README.md`
- Create: `terraform-registry-manifest.json`
- Create: `docs/index.md`
- Create: `docs/resources/project.md`
- Create: `examples/provider/provider.tf`
- Create: `examples/resources/openreplay_project/resource.tf`
- Create: `examples/resources/openreplay_project/import.sh`
- Create: `.gitignore`

**Interfaces:**
- Documents: `nitra/openreplay` provider configuration and `openreplay_project` schema from Tasks 1–3.

- [ ] **Step 1: Write a documentation completeness test or checklist assertion**

Confirm examples contain `source = "nitra/openreplay"`, a provider token variable, a `web` project, and an import command whose ID is numeric.

- [ ] **Step 2: Add README, registry manifest, provider/resource docs, examples, and Go `.gitignore`**

Document the owner/admin JWT requirement, environment variables, returned `project_key`, numeric ID import, and the intentionally deferred API-key and release automation work.

- [ ] **Step 3: Run formatting and test verification**

Run: `gofmt -w main.go internal && go test ./... && git diff --check`

Expected: all commands succeed with no formatting or whitespace errors.

- [ ] **Step 4: Commit**

```bash
git add README.md terraform-registry-manifest.json docs examples .gitignore main.go internal
git commit -m "docs: document OpenReplay project provider"
```

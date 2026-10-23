package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/nitra/terraform-provider-openreplay/internal/openreplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func projectResource(t *testing.T) resource.Resource {
	t.Helper()
	factories := New("test")().Resources(context.Background())
	require.Len(t, factories, 1, "provider must register its project resource")
	return factories[0]()
}

func resourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	var response resource.SchemaResponse
	projectResource(t).Schema(context.Background(), resource.SchemaRequest{}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	return response.Schema
}

func TestProjectResourceSchema(t *testing.T) {
	s := resourceSchema(t)
	require.Len(t, s.Attributes, 4, "resource state must only contain project fields")
	require.True(t, s.Attributes["id"].(schema.Int64Attribute).Computed)
	require.True(t, s.Attributes["project_key"].(schema.StringAttribute).Computed)
	require.True(t, s.Attributes["name"].(schema.StringAttribute).Required)
	require.True(t, s.Attributes["platform"].(schema.StringAttribute).Required)

	var metadata resource.MetadataResponse
	projectResource(t).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "openreplay"}, &metadata)
	require.Equal(t, "openreplay_project", metadata.TypeName)
}

func TestProjectResourceSchemaValidatesPlatform(t *testing.T) {
	platform := resourceSchema(t).Attributes["platform"].(schema.StringAttribute)
	for _, value := range []string{"web", "ios", "", "android", "Web", "IOS", " web", "web "} {
		t.Run(value, func(t *testing.T) {
			var response validator.StringResponse
			for _, validation := range platform.Validators {
				validation.ValidateString(context.Background(), validator.StringRequest{
					Path: path.Root("platform"), ConfigValue: types.StringValue(value),
				}, &response)
			}
			require.Equal(t, value != "web" && value != "ios", response.Diagnostics.HasError(), "%v", response.Diagnostics)
		})
	}
}

func TestProjectConfigureRejectsUnexpectedProviderData(t *testing.T) {
	configured, ok := projectResource(t).(resource.ResourceWithConfigure)
	require.True(t, ok, "project resource must consume the configured API client")
	var response resource.ConfigureResponse
	configured.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: struct{ Token string }{Token: "private-jwt"},
	}, &response)
	require.True(t, response.Diagnostics.HasError())
	for _, diagnostic := range response.Diagnostics {
		require.NotContains(t, diagnostic.Summary()+diagnostic.Detail(), "private-jwt")
	}
}

func TestProjectConfigureAllowsAbsentProviderData(t *testing.T) {
	configured, ok := projectResource(t).(resource.ResourceWithConfigure)
	require.True(t, ok)
	var response resource.ConfigureResponse
	configured.Configure(context.Background(), resource.ConfigureRequest{}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
}

func configuredProjectResource(t *testing.T, handler http.HandlerFunc) resource.Resource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := openreplay.NewClient(server.URL+"/api", "private-jwt")
	require.NoError(t, err)
	r := projectResource(t)
	var response resource.ConfigureResponse
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: client}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	return r
}

func projectState(t *testing.T, id, key, name, platform any) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Schema: resourceSchema(t),
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"id": tftypes.Number, "project_key": tftypes.String, "name": tftypes.String, "platform": tftypes.String,
		}}, map[string]tftypes.Value{
			"id":          tftypes.NewValue(tftypes.Number, id),
			"project_key": tftypes.NewValue(tftypes.String, key),
			"name":        tftypes.NewValue(tftypes.String, name),
			"platform":    tftypes.NewValue(tftypes.String, platform),
		}),
	}
}

func emptyProjectState(t *testing.T) tfsdk.State {
	t.Helper()
	s := resourceSchema(t)
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
}

func assertProjectState(t *testing.T, state tfsdk.State, id int64, key, name, platform string) {
	t.Helper()
	var gotID types.Int64
	require.False(t, state.GetAttribute(context.Background(), path.Root("id"), &gotID).HasError())
	require.Equal(t, types.Int64Value(id), gotID)
	for attribute, expected := range map[string]string{"project_key": key, "name": name, "platform": platform} {
		var got types.String
		require.False(t, state.GetAttribute(context.Background(), path.Root(attribute), &got).HasError())
		require.Equal(t, types.StringValue(expected), got, attribute)
	}
	require.NotContains(t, state.Raw.String(), "private-jwt")
}

func assertSafeProjectError(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	require.True(t, diagnostics.HasError(), "%v", diagnostics)
	for _, diagnostic := range diagnostics {
		require.NotContains(t, diagnostic.Summary()+diagnostic.Detail(), "private-jwt")
	}
}

func TestProjectCreateStoresServerProject(t *testing.T) {
	calls := 0
	r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
		calls++
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/api/projects", request.URL.Path)
		assert.Equal(t, "Bearer private-jwt", request.Header.Get("Authorization"))
		var body map[string]any
		assert.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		assert.Equal(t, map[string]any{"name": "Production", "platform": "web"}, body)
		_, _ = w.Write([]byte(`{"data":{"projectId":42,"projectKey":"created-key","name":"Production","platform":"web","saveRequestPayloads":true}}`))
	})
	plan := projectState(t, tftypes.UnknownValue, tftypes.UnknownValue, "Production", "web")
	response := resource.CreateResponse{State: emptyProjectState(t)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	require.Equal(t, 1, calls)
	assertProjectState(t, response.State, 42, "created-key", "Production", "web")
}

func TestProjectReadRefreshesAllProjectFields(t *testing.T) {
	calls := 0
	r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/api/projects/42", request.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"projectId":42,"projectKey":"refreshed-key","name":"Mobile","platform":"ios"}}`))
	})
	state := projectState(t, int64(42), "old-key", "Production", "web")
	response := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	require.Equal(t, 1, calls)
	assertProjectState(t, response.State, 42, "refreshed-key", "Mobile", "ios")
}

func TestProjectReadRemovesStateForMissingProject(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP 404", http.StatusNotFound, `{"detail":"missing"}`},
		{"absence envelope", http.StatusOK, `{"errors":["project not found"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
				calls++
				assert.Equal(t, "/api/projects/42", request.URL.Path)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			state := projectState(t, int64(42), "old-key", "Production", "web")
			response := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &response)
			require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
			require.Equal(t, 1, calls)
			require.True(t, response.State.Raw.IsNull(), "confirmed absence must remove the entire resource")
		})
	}
}

func TestProjectReadPreservesStateForOtherErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP 401", http.StatusUnauthorized, `{"detail":"private-jwt"}`},
		{"HTTP 403", http.StatusForbidden, `{"detail":"private-jwt"}`},
		{"unauthorized envelope", http.StatusOK, `{"errors":["unauthorized"]}`},
		{"untyped missing message", http.StatusInternalServerError, `{"detail":"project not found private-jwt"}`},
		{"invalid response", http.StatusOK, `{"data":"private-jwt"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := configuredProjectResource(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			state := projectState(t, int64(42), "old-key", "Production", "web")
			response := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &response)
			assertSafeProjectError(t, response.Diagnostics)
			require.True(t, state.Raw.Equal(response.State.Raw), "failed refresh must preserve prior state")
		})
	}
}

func TestProjectUpdateStoresServerProject(t *testing.T) {
	calls := 0
	r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
		calls++
		assert.Equal(t, http.MethodPut, request.Method)
		assert.Equal(t, "/api/projects/42", request.URL.Path)
		var body map[string]any
		assert.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		assert.Equal(t, map[string]any{"name": "Mobile", "platform": "ios"}, body)
		_, _ = w.Write([]byte(`{"data":{"projectId":42,"projectKey":"updated-key","name":"Mobile","platform":"ios"}}`))
	})
	state := projectState(t, int64(42), "old-key", "Production", "web")
	plan := projectState(t, tftypes.UnknownValue, tftypes.UnknownValue, "Mobile", "ios")
	response := resource.UpdateResponse{State: state}
	r.Update(context.Background(), resource.UpdateRequest{State: state, Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	require.Equal(t, 1, calls)
	assertProjectState(t, response.State, 42, "updated-key", "Mobile", "ios")
}

func TestProjectDeleteRemovesStateAfterSuccess(t *testing.T) {
	calls := 0
	r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
		calls++
		assert.Equal(t, http.MethodDelete, request.Method)
		assert.Equal(t, "/api/projects/42", request.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"state":"success"}}`))
	})
	state := projectState(t, int64(42), "old-key", "Production", "web")
	response := resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	require.Equal(t, 1, calls)
	require.True(t, response.State.Raw.IsNull())
}

func TestProjectMutationErrorsPreserveState(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			calls := 0
			r := configuredProjectResource(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = w.Write([]byte(`{"errors":["unauthorized","private-jwt"]}`))
			})
			state := projectState(t, int64(42), "old-key", "Production", "web")
			plan := projectState(t, tftypes.UnknownValue, tftypes.UnknownValue, "Mobile", "ios")
			switch operation {
			case "create":
				empty := emptyProjectState(t)
				response := resource.CreateResponse{State: empty}
				r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &response)
				assertSafeProjectError(t, response.Diagnostics)
				require.True(t, response.State.Raw.IsNull())
			case "update":
				response := resource.UpdateResponse{State: state}
				r.Update(context.Background(), resource.UpdateRequest{State: state, Plan: tfsdk.Plan{Schema: plan.Schema, Raw: plan.Raw}}, &response)
				assertSafeProjectError(t, response.Diagnostics)
				require.True(t, state.Raw.Equal(response.State.Raw))
			case "delete":
				response := resource.DeleteResponse{State: state}
				r.Delete(context.Background(), resource.DeleteRequest{State: state}, &response)
				assertSafeProjectError(t, response.Diagnostics)
				require.True(t, state.Raw.Equal(response.State.Raw))
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestProjectImportRejectsNonNumericID(t *testing.T) {
	for _, id := range []string{"", "project-42", "4.2", "9223372036854775808", "private-jwt"} {
		t.Run(id, func(t *testing.T) {
			calls := 0
			r := configuredProjectResource(t, func(http.ResponseWriter, *http.Request) { calls++ })
			importer, ok := r.(resource.ResourceWithImportState)
			require.True(t, ok, "project resource must support numeric import")
			response := resource.ImportStateResponse{State: emptyProjectState(t)}
			importer.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &response)
			assertSafeProjectError(t, response.Diagnostics)
			require.True(t, response.State.Raw.IsNull())
			require.Zero(t, calls, "invalid import must not contact the API")
		})
	}
}

func TestProjectImportStoresNumericIDBeforeRead(t *testing.T) {
	calls := 0
	r := configuredProjectResource(t, func(w http.ResponseWriter, request *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/api/projects/42", request.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"projectId":42,"projectKey":"imported-key","name":"Imported","platform":"web"}}`))
	})
	importer, ok := r.(resource.ResourceWithImportState)
	require.True(t, ok)
	response := resource.ImportStateResponse{State: emptyProjectState(t)}
	importer.ImportState(context.Background(), resource.ImportStateRequest{ID: "42"}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	require.Zero(t, calls, "import stores the ID; Read performs the refresh")
	var id types.Int64
	require.False(t, response.State.GetAttribute(context.Background(), path.Root("id"), &id).HasError())
	require.Equal(t, types.Int64Value(42), id)
	readResponse := resource.ReadResponse{State: response.State}
	r.Read(context.Background(), resource.ReadRequest{State: response.State}, &readResponse)
	require.False(t, readResponse.Diagnostics.HasError(), "%v", readResponse.Diagnostics)
	require.Equal(t, 1, calls)
	assertProjectState(t, readResponse.State, 42, "imported-key", "Imported", "web")
}

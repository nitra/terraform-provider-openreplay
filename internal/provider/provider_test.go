package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	framework "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/nitra/terraform-provider-openreplay/internal/openreplay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providerSchema(t *testing.T) schema.Schema {
	t.Helper()
	var response framework.SchemaResponse
	New("test")().Schema(context.Background(), framework.SchemaRequest{}, &response)
	require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
	return response.Schema
}

func configureProvider(t *testing.T, endpoint, token any) framework.ConfigureResponse {
	t.Helper()
	config := tfsdk.Config{
		Schema: providerSchema(t),
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"endpoint": tftypes.String, "token": tftypes.String,
		}}, map[string]tftypes.Value{
			"endpoint": tftypes.NewValue(tftypes.String, endpoint),
			"token":    tftypes.NewValue(tftypes.String, token),
		}),
	}
	var response framework.ConfigureResponse
	New("test")().Configure(context.Background(), framework.ConfigureRequest{Config: config}, &response)
	return response
}

func TestProviderSchemaMarksTokenSensitive(t *testing.T) {
	s := providerSchema(t)
	require.True(t, s.Attributes["token"].(schema.StringAttribute).Sensitive)
	require.True(t, s.Attributes["token"].IsOptional())
	require.True(t, s.Attributes["endpoint"].IsOptional())
}

func TestProviderSchemaRejectsEmptyToken(t *testing.T) {
	token := providerSchema(t).Attributes["token"].(schema.StringAttribute)
	var response validator.StringResponse
	for _, v := range token.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{
			Path: path.Root("token"), ConfigValue: types.StringValue(""),
		}, &response)
	}
	require.True(t, response.Diagnostics.HasError())
}

func TestProviderConfiguresFromEnvironment(t *testing.T) {
	for _, useHCL := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "HCL precedence"}[useHCL], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "/api/projects", r.URL.Path)
				assert.Equal(t, "Bearer selected-jwt", r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{"data":{"state":"success"}}`))
			}))
			defer server.Close()
			t.Setenv("OPENREPLAY_ENDPOINT", server.URL+"/api/")
			t.Setenv("OPENREPLAY_TOKEN", "selected-jwt")
			var endpoint, token any
			if useHCL {
				t.Setenv("OPENREPLAY_ENDPOINT", "https://unused.invalid/other")
				t.Setenv("OPENREPLAY_TOKEN", "unused-jwt")
				endpoint, token = server.URL+"/api", "selected-jwt"
			}
			response := configureProvider(t, endpoint, token)
			require.False(t, response.Diagnostics.HasError(), "%v", response.Diagnostics)
			require.Zero(t, calls, "Configure must not contact the API")
			client, ok := response.ResourceData.(*openreplay.Client)
			require.True(t, ok, "resource data must be the API client")
			require.NoError(t, client.Do(context.Background(), http.MethodGet, "/projects", nil, nil))
		})
	}
}

func TestProviderRejectsMissingInvalidAndUnknownConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		endpoint, token       any
		envEndpoint, envToken string
	}{
		{"missing both", nil, nil, "", ""},
		{"missing endpoint", nil, "private-jwt", "", ""},
		{"missing token", "https://example.test/api", nil, "", ""},
		{"empty HCL endpoint", "", "private-jwt", "https://example.test/api", "env-jwt"},
		{"empty HCL token", "https://example.test/api", "", "https://example.test/api", "env-jwt"},
		{"unknown endpoint", tftypes.UnknownValue, nil, "https://example.test/api", "env-jwt"},
		{"unknown token", nil, tftypes.UnknownValue, "https://example.test/api", "env-jwt"},
		{"endpoint credentials", "https://credential-secret@example.test/api", "private-jwt", "", ""},
		{"invalid URL", "https://credential-secret%zz@example.test/api", "private-jwt", "", ""},
		{"invalid endpoint path", "https://example.test/private-jwt", "private-jwt", "", ""},
		{"invalid token", "https://example.test/api", "private-jwt\nheader", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENREPLAY_ENDPOINT", tc.envEndpoint)
			t.Setenv("OPENREPLAY_TOKEN", tc.envToken)
			response := configureProvider(t, tc.endpoint, tc.token)
			require.True(t, response.Diagnostics.HasError())
			require.Nil(t, response.ResourceData)
			for _, diagnostic := range response.Diagnostics {
				require.NotContains(t, diagnostic.Summary()+diagnostic.Detail(), "private-jwt")
				require.NotContains(t, diagnostic.Summary()+diagnostic.Detail(), "credential-secret")
			}
		})
	}
}

func TestProviderMetadata(t *testing.T) {
	var response framework.MetadataResponse
	New("v0.1.0")().Metadata(context.Background(), framework.MetadataRequest{}, &response)
	require.Equal(t, "openreplay", response.TypeName)
	require.Equal(t, "v0.1.0", response.Version)
}

func TestProviderProtocol6Schema(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	require.NoError(t, err)
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	require.NoError(t, err)
	for _, diagnostic := range response.Diagnostics {
		require.NotEqual(t, tfprotov6.DiagnosticSeverityError, diagnostic.Severity, "%s: %s", diagnostic.Summary, diagnostic.Detail)
	}
	require.NotNil(t, response.Provider)
}

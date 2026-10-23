// Package provider implements the OpenReplay Terraform Plugin Framework provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	framework "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nitra/terraform-provider-openreplay/internal/openreplay"
)

var _ framework.Provider = (*OpenReplayProvider)(nil)

// OpenReplayProvider configures access to the OpenReplay dashboard API.
type OpenReplayProvider struct {
	version string
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

// New returns a provider factory with the supplied build version.
func New(version string) func() framework.Provider {
	return func() framework.Provider {
		return &OpenReplayProvider{version: version}
	}
}

// Metadata identifies the provider and its resource type prefix.
func (p *OpenReplayProvider) Metadata(_ context.Context, _ framework.MetadataRequest, response *framework.MetadataResponse) {
	response.TypeName = "openreplay"
	response.Version = p.version
}

// Schema describes provider configuration, which is never resource state.
func (p *OpenReplayProvider) Schema(_ context.Context, _ framework.SchemaRequest, response *framework.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Manage projects through the authenticated OpenReplay dashboard API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Absolute HTTP(S) dashboard API URL ending in /api. Defaults to OPENREPLAY_ENDPOINT.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Owner/admin dashboard JWT. Defaults to OPENREPLAY_TOKEN.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

// Configure resolves HCL/environment values and supplies the client to resources.
func (p *OpenReplayProvider) Configure(ctx context.Context, request framework.ConfigureRequest, response *framework.ConfigureResponse) {
	var config providerModel
	response.Diagnostics.Append(request.Config.Get(ctx, &config)...)
	if response.Diagnostics.HasError() {
		return
	}
	if config.Endpoint.IsUnknown() {
		response.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown OpenReplay Endpoint", "The endpoint must be known before configuring the provider.")
	}
	if config.Token.IsUnknown() {
		response.Diagnostics.AddAttributeError(path.Root("token"), "Unknown OpenReplay Token", "The dashboard JWT must be known before configuring the provider.")
	}
	if response.Diagnostics.HasError() {
		return
	}

	// Framework provider schemas have no DefaultFunc; resolve environment defaults
	// here only for null values so explicit HCL values always take precedence.
	endpoint := config.Endpoint.ValueString()
	if config.Endpoint.IsNull() {
		endpoint = os.Getenv("OPENREPLAY_ENDPOINT")
	}
	token := config.Token.ValueString()
	if config.Token.IsNull() {
		token = os.Getenv("OPENREPLAY_TOKEN")
	}
	if endpoint == "" {
		response.Diagnostics.AddAttributeError(path.Root("endpoint"), "Missing OpenReplay Endpoint", "Set endpoint in the provider configuration or OPENREPLAY_ENDPOINT in the environment.")
	}
	if token == "" {
		response.Diagnostics.AddAttributeError(path.Root("token"), "Missing OpenReplay Token", "Set token to an owner/admin dashboard JWT or set OPENREPLAY_TOKEN in the environment.")
	}
	if response.Diagnostics.HasError() {
		return
	}
	client, err := openreplay.NewClient(endpoint, token)
	if err != nil {
		response.Diagnostics.AddError("Invalid OpenReplay Configuration", err.Error())
		return
	}
	response.ResourceData = client
	response.DataSourceData = client
}

// Resources returns the resource factories supported by this provider.
func (p *OpenReplayProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewProjectResource}
}

// DataSources returns the data source factories supported by this provider.
func (p *OpenReplayProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

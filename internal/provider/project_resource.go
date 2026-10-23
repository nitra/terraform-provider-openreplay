package provider

import (
	"context"
	"errors"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nitra/terraform-provider-openreplay/internal/openreplay"
)

var _ resource.Resource = (*ProjectResource)(nil)
var _ resource.ResourceWithConfigure = (*ProjectResource)(nil)
var _ resource.ResourceWithImportState = (*ProjectResource)(nil)

// ProjectResource manages a project through the OpenReplay dashboard API.
type ProjectResource struct {
	client *openreplay.Client
}

type projectResourceModel struct {
	ID         types.Int64  `tfsdk:"id"`
	ProjectKey types.String `tfsdk:"project_key"`
	Name       types.String `tfsdk:"name"`
	Platform   types.String `tfsdk:"platform"`
}

// NewProjectResource returns a project resource with its own client configuration.
func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

// Metadata identifies the resource under the provider's type prefix.
func (r *ProjectResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_project"
}

// Schema describes the managed project fields and server-generated identifiers.
func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		Description: "Manage an OpenReplay project through the authenticated dashboard API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "Numeric dashboard project ID.",
			},
			"project_key": schema.StringAttribute{
				Computed:    true,
				Description: "Project key used to configure the OpenReplay tracker.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Project name.",
			},
			"platform": schema.StringAttribute{
				Required:    true,
				Description: "Project platform: web or ios.",
				Validators:  []validator.String{stringvalidator.OneOf("web", "ios")},
			},
		},
	}
}

// Configure receives the API client from provider configuration.
func (r *ProjectResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}
	client, ok := request.ProviderData.(*openreplay.Client)
	if !ok || client == nil {
		response.Diagnostics.AddError("Unexpected OpenReplay Resource Configuration", "The provider did not supply an OpenReplay API client. Report this issue to the provider developer.")
		return
	}
	r.client = client
}

// Create creates the project and saves its returned fields.
func (r *ProjectResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan projectResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}
	project, err := r.client.CreateProject(ctx, openreplay.ProjectInput{
		Name: plan.Name.ValueString(), Platform: plan.Platform.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError("Unable to Create OpenReplay Project", err.Error())
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, projectModelFromAPI(project))...)
}

// Read refreshes the project and removes confirmed missing projects from state.
func (r *ProjectResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var id types.Int64
	response.Diagnostics.Append(request.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if response.Diagnostics.HasError() {
		return
	}
	project, err := r.client.GetProject(ctx, id.ValueInt64())
	if errors.Is(err, openreplay.ErrNotFound) {
		response.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		response.Diagnostics.AddError("Unable to Read OpenReplay Project", err.Error())
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, projectModelFromAPI(project))...)
}

// Update updates the mutable project fields.
func (r *ProjectResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var plan projectResourceModel
	var id types.Int64
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if response.Diagnostics.HasError() {
		return
	}
	project, err := r.client.UpdateProject(ctx, id.ValueInt64(), openreplay.ProjectInput{
		Name: plan.Name.ValueString(), Platform: plan.Platform.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError("Unable to Update OpenReplay Project", err.Error())
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, projectModelFromAPI(project))...)
}

// Delete deletes the remote project before removing its state.
func (r *ProjectResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var id types.Int64
	response.Diagnostics.Append(request.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if response.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteProject(ctx, id.ValueInt64()); err != nil {
		response.Diagnostics.AddError("Unable to Delete OpenReplay Project", err.Error())
		return
	}
	response.State.RemoveResource(ctx)
}

// ImportState validates and stores a numeric ID for the subsequent Read.
func (r *ProjectResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(request.ID, 10, 64)
	if err != nil {
		response.Diagnostics.AddError("Invalid OpenReplay Project Import ID", "The import ID must be a decimal integer within the signed 64-bit range, such as 42.")
		return
	}
	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// projectModelFromAPI maps only managed project fields into Terraform state.
func projectModelFromAPI(project openreplay.Project) projectResourceModel {
	return projectResourceModel{
		ID:         types.Int64Value(project.ID),
		ProjectKey: types.StringValue(project.Key),
		Name:       types.StringValue(project.Name),
		Platform:   types.StringValue(project.Platform),
	}
}

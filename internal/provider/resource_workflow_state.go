package provider

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &WorkflowStateResource{}
var _ resource.ResourceWithImportState = &WorkflowStateResource{}

func NewWorkflowStateResource() resource.Resource {
	return &WorkflowStateResource{}
}

type WorkflowStateResource struct {
	client *graphql.Client
}

type WorkflowStateResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
	Color       types.String `tfsdk:"color"`
	Position    types.Number `tfsdk:"position"`
	TeamId      types.String `tfsdk:"team_id"`
}

func (r *WorkflowStateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow_state"
}

func (r *WorkflowStateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Linear team workflow state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the workflow state.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the workflow state.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.UTF8LengthAtLeast(1),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of the workflow state.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf([]string{"triage", "backlog", "unstarted", "started", "completed", "canceled"}...),
				},
			},
			"position": schema.NumberAttribute{
				MarkdownDescription: "Position of the workflow state.",
				Required:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Color of the workflow state.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(colorRegex(), "must be a hex color"),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description of the workflow state.",
				Optional:            true,
			},
			"team_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the team.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidRegex(), "must be an uuid"),
				},
			},
		},
	}
}

func (r *WorkflowStateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*graphql.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *graphql.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *WorkflowStateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *WorkflowStateResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	position, _ := data.Position.ValueBigFloat().Float64()

	input := WorkflowStateCreateInput{
		Name:        data.Name.ValueString(),
		Type:        data.Type.ValueString(),
		Position:    position,
		Color:       data.Color.ValueString(),
		Description: data.Description.ValueStringPointer(),
		TeamId:      data.TeamId.ValueString(),
	}

	response, err := createWorkflowState(ctx, *r.client, input)

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create workflow state, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "created a workflow state")

	workflowState := response.WorkflowStateCreate.WorkflowState

	data.Id = types.StringValue(workflowState.Id)
	data.Name = types.StringValue(workflowState.Name)
	data.Type = types.StringValue(workflowState.Type)
	data.Position = types.NumberValue(big.NewFloat(workflowState.Position))
	data.Color = types.StringValue(workflowState.Color)
	data.Description = types.StringPointerValue(workflowState.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowStateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *WorkflowStateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	response, err := getWorkflowState(ctx, *r.client, data.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow state, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "read a workflow state")

	workflowState := response.WorkflowState

	data.Name = types.StringValue(workflowState.Name)
	data.Type = types.StringValue(workflowState.Type)
	data.Position = types.NumberValue(big.NewFloat(workflowState.Position))
	data.Color = types.StringValue(workflowState.Color)
	data.TeamId = types.StringValue(workflowState.Team.Id)
	data.Description = types.StringPointerValue(workflowState.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowStateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *WorkflowStateResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	position, _ := data.Position.ValueBigFloat().Float64()

	input := WorkflowStateUpdateInput{
		Name:        data.Name.ValueString(),
		Color:       data.Color.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Position:    position,
	}

	response, err := updateWorkflowState(ctx, *r.client, input, data.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update workflow state, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "updated a workflow state")

	workflowState := response.WorkflowStateUpdate.WorkflowState

	data.Name = types.StringValue(workflowState.Name)
	data.Position = types.NumberValue(big.NewFloat(workflowState.Position))
	data.Color = types.StringValue(workflowState.Color)
	data.Description = types.StringPointerValue(workflowState.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowStateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *WorkflowStateResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	_, err := deleteWorkflowState(ctx, *r.client, data.Id.ValueString())

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete workflow state, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted a workflow state")
}

func (r *WorkflowStateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: workflow_state_name:team_key. Got: %q", req.ID),
		)

		return
	}

	response, err := findWorkflowState(ctx, *r.client, parts[0], parts[1])

	if err != nil || len(response.WorkflowStates.Nodes) != 1 {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to import workflow state, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), response.WorkflowStates.Nodes[0].Id)...)
}

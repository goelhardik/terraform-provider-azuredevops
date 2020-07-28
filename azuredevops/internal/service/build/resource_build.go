package build

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/microsoft/azure-devops-go-api/azuredevops/build"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/client"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils/suppress"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils/tfhelper"
)

func ResourceBuild() *schema.Resource {
	return &schema.Resource{
		Create: resourceBuildCreate,
		Read:   resourceBuildRead,
		Update: resourceBuildUpdate,
		Delete: resourceBuildDelete,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.IsUUID,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"definition_id": {
				Type:         schema.TypeInt,
				Required:     true,
				ValidateFunc: validation.IntAtLeast(1),
			},
			"source_branch": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotEmpty,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"url": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceBuildCreate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	source_branch := d.Get("source_branch").(string)
	definition_id := d.Get("definition_id").(int)
	projectID := d.Get("project_id").(string)

	queueBuildArgs := build.QueueBuildArgs{
		Build: &build.Build{
			Definition: &build.DefinitionReference{
				Id: &definition_id,
			},
			SourceBranch: &source_branch,
		},
		Project: &projectID,
	}
	build, err := clients.BuildClient.QueueBuild(clients.Ctx, queueBuildArgs)
	if err != nil {
		return fmt.Errorf("Error queuing build in Azure DevOps: %+v", err)
	}
	d.SetId(strconv.Itoa(*build.Id))

	return resourceBuildRead(d, m)
}

func resourceBuildRead(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	projectID, buildId, err := tfhelper.ParseProjectIDAndResourceID(d)
	if err != nil {
		return err
	}

	getBuildArgs := build.GetBuildArgs{
		Project: &projectID,
		BuildId: &buildId,
	}
	build, err := clients.BuildClient.GetBuild(clients.Ctx, getBuildArgs)
	if err != nil {
		return fmt.Errorf("Error fetching build %d in Azure DevOps: %+v", buildId, err)
	}
	err = flattenBuild(d, build)
	if err != nil {
		return fmt.Errorf("Error flattening build: %w", err)
	}

	return nil
}

func resourceBuildUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceBuildRead(d, m)
}

func resourceBuildDelete(d *schema.ResourceData, m interface{}) error {
	return nil
}

func flattenBuild(d *schema.ResourceData, build *build.Build) error {
	d.SetId(strconv.Itoa(*build.Id))
	d.Set("url", build.Url)

	return nil
}

package git

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"

	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/client"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils/suppress"
)

func ResourceGitPullRequest() *schema.Resource {
	return &schema.Resource{
		Create: resourceGitPullRequestCreate,
		Read:   resourceGitPullRequestRead,
		Update: resourceGitPullRequestUpdate,
		Delete: resourceGitPullRequestDelete,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.IsUUID,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"repo_name": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotWhiteSpace,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"title": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsNotWhiteSpace,
			},
			"source_ref_name": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotWhiteSpace,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"target_ref_name": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotWhiteSpace,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"url": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceGitPullRequestCreate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	projectID := d.Get("project_id").(string)
	repoName := d.Get("repo_name").(string)
	title := d.Get("title").(string)
	sourceRefName := d.Get("source_ref_name").(string)
	targetRefName := d.Get("target_ref_name").(string)
	description := d.Get("description").(string)

	createPullRequestArgs := git.CreatePullRequestArgs{
		Project:      &projectID,
		RepositoryId: &repoName,
		GitPullRequestToCreate: &git.GitPullRequest{
			Title:         &title,
			Description:   &description,
			SourceRefName: &sourceRefName,
			TargetRefName: &targetRefName,
		},
	}
	pr, err := clients.GitReposClient.CreatePullRequest(clients.Ctx, createPullRequestArgs)
	if err != nil {
		return fmt.Errorf("error creating resource Pull Request: %+v", err)
	}

	flattenPullRequest(d, pr)

	return resourceGitPullRequestRead(d, m)
}

func resourceGitPullRequestRead(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	projectID := d.Get("project_id").(string)
	prId, err := strconv.Atoi(d.Id())

	prIdArgs := git.GetPullRequestByIdArgs{
		PullRequestId: &prId,
		Project:       &projectID,
	}
	pr, err := clients.GitReposClient.GetPullRequestById(clients.Ctx, prIdArgs)
	if err != nil {
		return fmt.Errorf("Error fetching build %d in Azure DevOps: %+v", prId, err)
	}
	err = flattenPullRequest(d, pr)
	if err != nil {
		return fmt.Errorf("Error flattening build: %w", err)
	}

	return nil
}

func resourceGitPullRequestUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceGitPullRequestRead(d, m)
}

func resourceGitPullRequestDelete(d *schema.ResourceData, m interface{}) error {
	return nil
}

func flattenPullRequest(d *schema.ResourceData, pr *git.GitPullRequest) error {
	d.SetId(strconv.Itoa(*pr.PullRequestId))
	d.Set("url", pr.Url)

	return nil
}

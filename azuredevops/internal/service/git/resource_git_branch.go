package git

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/client"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils/converter"
	"github.com/terraform-providers/terraform-provider-azuredevops/azuredevops/internal/utils/suppress"
)

func ResourceGitBranch() *schema.Resource {
	return &schema.Resource{
		Create: resourceGitBranchCreate,
		Read:   resourceGitBranchRead,
		Update: resourceGitBranchUpdate,
		Delete: resourceGitBranchDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:             schema.TypeString,
				ForceNew:         true, // branches cannot be renamed
				Required:         true,
				ValidateFunc:     validation.NoZeroValues,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"repo_name": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotWhiteSpace,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"project_id": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.IsUUID,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"old_object_id": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotEmpty,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"new_object_id": {
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsNotEmpty,
				DiffSuppressFunc: suppress.CaseDifference,
			},
			"content": {
				Type:         schema.TypeString,
				ValidateFunc: validation.StringIsNotEmpty,
				Optional:     true,
			},
			"root_path": {
				Type:         schema.TypeString,
				ValidateFunc: validation.StringIsNotEmpty,
				Optional:     true,
			},
			"url": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"object_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceGitBranchCreate(d *schema.ResourceData, m interface{}) error {
	clients := m.(*client.AggregatedClient)

	name := d.Get("name").(string)
	repoName := d.Get("repo_name").(string)
	projectID := d.Get("project_id").(string)
	oldObjectID := d.Get("old_object_id").(string)
	newObjectID := d.Get("new_object_id").(string)
	content := d.Get("content").(string)
	rootPath := d.Get("root_path").(string)
	branch := &git.GitRefUpdate{
		Name:        &name,
		OldObjectId: &oldObjectID,
		NewObjectId: &newObjectID,
	}

	refResult, err := createGitBranch(clients, branch, &repoName, &projectID, &content, &rootPath)
	if err != nil {
		return fmt.Errorf("Error creating branch in Azure DevOps: %+v", err)
	}

	if content != "" {
		err = scaffoldFiles(clients, &content, &rootPath, &projectID, &repoName, refResult.Name, refResult.NewObjectId)
		if err != nil {
			return fmt.Errorf("Error creating branch in Azure DevOps: %+v", err)
		}
	}

	return resourceGitBranchRead(d, m)
}

func resourceGitBranchRead(d *schema.ResourceData, m interface{}) error {
	name := d.Get("name").(string)
	repoName := d.Get("repo_name").(string)
	projectID := d.Get("project_id").(string)

	clients := m.(*client.AggregatedClient)
	branch, err := gitBranchRead(clients, name, repoName, projectID)
	if err != nil {
		if utils.ResponseWasNotFound(err) {
			return nil
		}
		return fmt.Errorf("Error looking up branch with Name %s in Repo %s. Error: %v", name, repoName, err)
	}
	if branch == nil {
		d.SetId("")
		return nil
	}
	err = flattenGitBranch(d, branch)
	if err != nil {
		return fmt.Errorf("Error flattening Git branch: %w", err)
	}
	return nil
}

func resourceGitBranchUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceGitBranchRead(d, m)
}

func resourceGitBranchDelete(d *schema.ResourceData, m interface{}) error {
	name := d.Get("name").(string)
	repoName := d.Get("repo_name").(string)
	projectID := d.Get("project_id").(string)
	content := d.Get("content").(string)
	rootPath := d.Get("root_path").(string)

	clients := m.(*client.AggregatedClient)
	branch, err := gitBranchRead(clients, name, repoName, projectID)
	if err != nil {
		if utils.ResponseWasNotFound(err) {
			return nil
		}
		return fmt.Errorf("Error looking up branch with Name %s in Repo %s. Error: %v", name, repoName, err)
	}

	// delete branch
	deletedObjectId := "0000000000000000000000000000000000000000"
	branchUpdate := &git.GitRefUpdate{
		Name:        &name,
		OldObjectId: branch.ObjectId,
		NewObjectId: &deletedObjectId,
	}

	_, err = createGitBranch(clients, branchUpdate, &repoName, &projectID, &content, &rootPath)
	if err != nil {
		return fmt.Errorf("Error deleting branch in Azure DevOps: %+v", err)
	}
	d.SetId("")

	return nil
}

func createGitBranch(clients *client.AggregatedClient, branch *git.GitRefUpdate, repoName *string, projectID *string, contentFile *string, rootPath *string) (*git.GitRefUpdateResult, error) {
	args := git.UpdateRefsArgs{
		RefUpdates:   &[]git.GitRefUpdate{*branch},
		RepositoryId: repoName,
		Project:      projectID,
	}
	updateRefsResult, err := clients.GitReposClient.UpdateRefs(clients.Ctx, args)
	if err != nil {
		return nil, err
	}

	refResult := (*updateRefsResult)[0]
	if *refResult.Success != true {
		return nil, fmt.Errorf("Branch creation failed due to %s", refResult.CustomMessage)
	}

	return &refResult, nil
}

func scaffoldFiles(clients *client.AggregatedClient, contentFile *string, rootPath *string, projectID *string, repoName *string, branchName *string, branchObjectID *string) error {
	// scaffold content
	var changes []interface{}
	commitMessage := "Scaffolding content"
	basePath := *contentFile
	_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("Unable to read file %s", path)
		}
		base64Content := base64.StdEncoding.EncodeToString(content)
		relativePath, err := filepath.Rel(basePath, path)
		if err != nil {
			return fmt.Errorf("Unable to get relative path for file %s", path)
		}
		relativePath = *rootPath + "/" + relativePath

		change := &Change{
			ChangeType: &git.VersionControlChangeTypeValues.Add,
			Item: &ChangeItem{
				Path: &relativePath,
			},
			NewContent: &git.ItemContent{
				Content:     &base64Content,
				ContentType: &git.ItemContentTypeValues.Base64Encoded,
			},
		}
		changes = append(changes, change)

		return nil
	})

	pushArgs := git.CreatePushArgs{
		Project:      projectID,
		RepositoryId: repoName,
		Push: &git.GitPush{
			RefUpdates: &[]git.GitRefUpdate{
				{
					Name:        branchName,
					OldObjectId: branchObjectID,
				},
			},
			Commits: &[]git.GitCommitRef{
				{
					Comment: &commitMessage,
					Changes: &changes,
				},
			},
		},
	}
	_, err := clients.GitReposClient.CreatePush(clients.Ctx, pushArgs)
	if err != nil {
		return err
	}

	return nil
}

// Lookup an Azure Git branch using the name.
func gitBranchRead(clients *client.AggregatedClient, branchName string, repoName string, projectID string) (*git.GitRef, error) {
	var branch *git.GitRef
	var err error

	getRefsResponse, err := clients.GitReposClient.GetRefs(clients.Ctx, git.GetRefsArgs{
		// FilterContains: &branchName,
		RepositoryId: converter.String(repoName),
		Project:      converter.String(projectID),
	})
	if err != nil {
		return nil, err
	}
	if branchName != "" {
		for _, ref := range getRefsResponse.Value {
			if strings.EqualFold(*ref.Name, branchName) {
				branch = &ref
				return branch, err
			}
		}
	}

	return nil, nil
}

func flattenGitBranch(d *schema.ResourceData, branch *git.GitRef) error {
	d.SetId(*branch.Url)
	d.Set("name", branch.Name)
	d.Set("object_id", branch.ObjectId)
	d.Set("creator", branch.Creator)
	d.Set("url", branch.Url)

	return nil
}

type Change struct {
	// The type of change that was made to the item.
	ChangeType *git.VersionControlChangeType `json:"changeType,omitempty"`
	// Current version.
	Item *ChangeItem `json:"item,omitempty"`
	// Content of the item after the change.
	NewContent *git.ItemContent `json:"newContent,omitempty"`
	// Path of the item on the server.
	SourceServerItem *string `json:"sourceServerItem,omitempty"`
	// URL to retrieve the item.
	Url *string `json:"url,omitempty"`
}

type ChangeItem struct {
	Path *string `json:"path,omitempty"`
}

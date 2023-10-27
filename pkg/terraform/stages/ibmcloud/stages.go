package ibmcloud

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/pkg/errors"

	"github.com/openshift/installer/pkg/terraform"
	"github.com/openshift/installer/pkg/terraform/providers"
	"github.com/openshift/installer/pkg/terraform/stages"
	ibmcloudtfvars "github.com/openshift/installer/pkg/tfvars/ibmcloud"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
)

// PlatformStages are the stages to run to provision the infrastructure in IBM Cloud.
var PlatformStages = []terraform.Stage{
	stages.NewStage(
		"ibmcloud",
		"network",
		[]providers.Provider{providers.IBM},
	),
	stages.NewStage(
		"ibmcloud",
		"bootstrap",
		[]providers.Provider{providers.IBM},
		stages.WithCustomBootstrapDestroy(customBootstrapDestroy),
	),
	stages.NewStage(
		"ibmcloud",
		"master",
		[]providers.Provider{providers.IBM},
	),
}

func customBootstrapDestroy(s stages.SplitStage, directory string, terraformDir string, varFiles []string) error {
	// IBM Cloud Service endpoints can be overridden, so we must make sure any specified are used during BootstrapDestroy
	opts := make([]tfexec.DestroyOption, 0, len(varFiles) + 1)
	for _, varFile := range varFiles {
		opts = append(opts, tfexec.VarFile(varFile))
	}

	// If there is a service override JSON file in the terraformDir, we want to inject that file into the Terraform variable 'ibmcloud_endpoints_json_file'
	if _, err := os.Stat(filepath.Join(terraformDir, ibmcloudtfvars.IBMCloudEndpointJSONFileName)); err == nil {
		opts = append(opts, tfexec.Var(fmt.Sprintf("ibmcloud_endpoints_json_file=%s", filepath.Join(terraformDir, ibmcloudtfvars.IBMCloudEndpointJSONFileName))))
	}

	return errors.Wrap(
		terraform.Destroy(directory, ibmcloudtypes.Name, s, terraformDir, opts...),
		"failed to destroy bootstrap",
	)
}

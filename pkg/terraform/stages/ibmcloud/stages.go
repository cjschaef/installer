package ibmcloud

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

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
	opts := make([]tfexec.DestroyOption, 0, len(varFiles)+1)
	for _, varFile := range varFiles {
		opts = append(opts, tfexec.VarFile(varFile))
	}

	// If there is a service override JSON file in the directory, we want to inject that file into the Terraform variables.
	terraformParentDir := filepath.Dir(terraformDir)
	logrus.Infof("terraform parent directory: %s", terraformParentDir)
	endpointOverrideFile := filepath.Join(terraformParentDir, ibmcloudtfvars.IBMCloudEndpointJSONFileName)
	logrus.Infof("checking for endpoint override file: %s", endpointOverrideFile)
	logrus.Infof("terraform directory: %s", terraformDir)
	if _, err := os.Stat(endpointOverrideFile); err == nil {
		// Set variable to use private endpoints via 'ibmcloud_endpoints_json_file' override JSON data
		logrus.Info("configuring terraform for ibm endpoint override")
		opts = append(opts, tfexec.Var(fmt.Sprintf("ibmcloud_endpoints_json_file=%s", endpointOverrideFile)))
	}

	return errors.Wrap(terraform.Destroy(directory, ibmcloudtypes.Name, s, terraformDir, opts...), "failed to destroy bootstrap")
}

// Package bootstrap uses Terraform to remove bootstrap resources.
package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/openshift/installer/pkg/asset/cluster"
	openstackasset "github.com/openshift/installer/pkg/asset/cluster/openstack"
	osp "github.com/openshift/installer/pkg/destroy/openstack"
	infra "github.com/openshift/installer/pkg/infrastructure/platform"
	typesazure "github.com/openshift/installer/pkg/types/azure"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
	"github.com/openshift/installer/pkg/types/openstack"
	ibmcloudtfvars "github.com/openshift/installer/pkg/tfvars/ibmcloud"
)

// Destroy uses Terraform to remove bootstrap resources.
func Destroy(dir string) (err error) {
	metadata, err := cluster.LoadMetadata(dir)
	if err != nil {
		return err
	}

	platform := metadata.Platform()
	if platform == "" {
		return errors.New("no platform configured in metadata")
	}

	if platform == openstack.Name {
		if err := openstackasset.PreTerraform(); err != nil {
			return errors.Wrapf(err, "Failed to  initialize infrastructure")
		}

		imageName := metadata.InfraID + "-ignition"
		if err := osp.DeleteGlanceImage(imageName, metadata.OpenStack.Cloud); err != nil {
			return errors.Wrapf(err, "Failed to delete glance image %s", imageName)
		}
	}

	// Azure Stack uses the Azure platform but has its own Terraform configuration.
	if platform == typesazure.Name && metadata.Azure.CloudName == typesazure.StackCloud {
		platform = typesazure.StackTerraformName
	}

	// IBM Cloud allows overrides of service endpoints, possibly required during bootstrap destroy
	// create a JSON file with overrides, if any are present
	if platform == ibmcloudtypes.Name {
		if metadata.IBMCloud != nil && len(metadata.IBMCloud.ServiceEndpoints) > 0 {
			// Build the JSON containing endpoint overrides for IBM Cloud Services
			jsonData, err := ibmcloudtfvars.CreateEndpointJSON(metadata.IBMCloud.ServiceEndpoints, metadata.IBMCloud.Region)
			if err != nil {
				return errors.Wrap(err, "failed to create IBM Cloud service endpoint override JSON for bootstrap destroy")
			}

			if jsonData != nil {
				// If JSON data was generated, create the JSON file in the destroy directory, for IBM Cloud Terraform provider to use
				// This is placed in the 'terraform' directory, so it is accessible during the custom Destroy logic
				if err := os.WriteFile(filepath.Join(dir, "terraform", ibmcloudtfvars.IBMCloudEndpointJSONFileName), jsonData, 0o640); err != nil {
					return errors.Wrap(err, "failed to write IBM Cloud service endpoint override JSON file for bootstrap destroy")
				}
			}
		}
	}

	provider := infra.ProviderForPlatform(platform)
	if err := provider.DestroyBootstrap(dir); err != nil {
		return fmt.Errorf("error destroying bootstrap resources %w", err)
	}

	return nil
}

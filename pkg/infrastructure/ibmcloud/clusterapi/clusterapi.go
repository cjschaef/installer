package clusterapi

import (
	"context"
	"fmt"

	ibmcloudic "github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
	"github.com/openshift/installer/pkg/infrastructure/clusterapi"
	"github.com/openshift/installer/pkg/rhcos/cache"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
)

var _ clusterapi.PreProvider = (*Provider)(nil)
var _ clusterapi.Provider = (*Provider)(nil)

// Provider implements IBM Cloud CAPI installation.
type Provider struct{}

// Name returns the IBM Cloud provider name.
func (p Provider) Name() string {
	return ibmcloudtypes.Name
}

// PreProvision creates the IBM Cloud objects required prior to running capibmcloud.
func (p Provider) PreProvision(ctx context.Context, in clusterapi.PreProvisionInput) error {
	// Before Provisioning IBM Cloud Infrastructure for the Cluster, we must perform the following.
	// 1. Create the Resource Group to house cluster resources, if necessary (BYO RG).
	// 2. Create a COS Instance and Bucket to host the RHCOS Custom Image file.
	// 3. Upload the RHCOS image to the COS Bucket.

	// Setup IBM Cloud Client.
	metadata := ibmcloudic.NewMetadata(in.InstallConfig.Config)
	client, err := metadata.Client()
	if err != nil {
		return fmt.Errorf("failed creating IBM Cloud client: %w", err)
	}

	// Create cluster's Resource Group, if necessary (BYO RG is supported).
	resourceGroupName := in.InfraID
	if in.InstallConfig.Config.Platform.IBMCloud.ResourceGroupName != "" {
		resourceGroupName = in.InstallConfig.Config.Platform.IBMCloud.ResourceGroupName
	}

	// Check whether the Resource Group already exists.
	resourceGroup, err := client.GetResourceGroup(ctx, resourceGroupName)
	if err != nil {
		// If Resource Group cannot be found, but it was provided in install-config (use existing RG), raise an error.
		// We could create the Resource Group, defined by user, but that will make Resource cleanup more difficult.
		if in.InstallConfig.Config.Platform.IBMCloud.ResourceGroupName != "" {
			return fmt.Errorf("provided resource group not found: %w", err)
		}
	}

	// Create Resource Group if it wasn't found (and was provided as existing RG).
	if resourceGroup == nil {
		err := client.CreateResourceGroup(ctx, resourceGroupName)
		if err != nil {
			return fmt.Errorf("failed creating new resource group: %w", err)
		}
	}

	// Create a COS Instance and Bucket to host the RHCOS image file.
	// NOTE(cjschaef): Support to use an existing COS Object (RHCO image file) or VPC Custom Image could be added to skip this step.
	cosInstanceName := fmt.Sprintf("%s-cos", in.InfraID)
	cosInstance, err := client.CreateCOSInstance(ctx, cosInstanceName, *resourceGroup.ID)
	if err != nil {
		return fmt.Errorf("failed creating RHCOS image COS instance: %w", err)
	}
	bucketName := fmt.Sprintf("%s-vsi-imge", in.InfraID)
	err = client.CreateCOSBucket(ctx, *cosInstance.ID, bucketName)
	if err != nil {
		return fmt.Errorf("failed creating RHCOS image COS bucket: %w", err)
	}

	// Upload the RHCOS image to the COS Bucket.
	cachedImage, err := cache.DownloadImageFile(string(*in.RhcosImage), cache.InstallerApplicationName)
	if err != nil {
		return fmt.Errorf("failed to use cached ibmcloud image: %w", err)
	}
	err = client.CreateCOSObject(ctx, cachedImage, *cosInstance.ID, bucketName)
	if err != nil {
		return fmt.Errorf("failed uploading RHCOS image: %w", err)
	}

	// NOTE(cjschaef): We may need to create an IAM Authorization policy for VPC to COS Reader access, for when the Custom Image is created using the COS Object above.
	return nil
}

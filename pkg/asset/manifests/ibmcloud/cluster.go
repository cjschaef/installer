package ibmcloud

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/IBM/go-sdk-core/v5/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	ibmcloudic "github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
)

// GenerateClusterAssets generates the manifests for the cluster-api.
func GenerateClusterAssets(installConfig *installconfig.InstallConfig, clusterID *installconfig.ClusterID, imageName string) (*capiutils.GenerateClusterAssetsOutput, error) {
	manifests := []*asset.RuntimeFile{}
	// TODO(cjschaef): Add support for creating VPC Subnet Address Pools (CIDRs) during Infrastructure bring up
	// mainCIDR := capiutils.CIDRFromInstallConfig(installConfig)
	platform := installConfig.Config.Platform.IBMCloud
	// Make sure we have a fresh instance of Metadata, in case of any service endpoint overrides
	metadata := ibmcloudic.NewMetadata(installConfig.Config)
	client, err := metadata.Client()
	if err != nil {
		return nil, fmt.Errorf("failed creating IBM Cloud client %w", err)
	}
	operatingSystem := "rhel-coreos-stable-amd64"

	// Create IBM Cloud Credentials for IBM Cloud CAPI
	ibmcloudCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-credentials",
			Namespace: capiutils.Namespace,
		},
		Data: make(map[string][]byte),
	}
	ibmcloudCreds.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))

	// TODO(cjschaef): Determine whether we need these credentials or will rely on env var's for CAPI
	// Encode the API key prior to adding it to secret data
	encodedAPIKey := make([]byte, base64.StdEncoding.EncodedLen(len(os.Getenv("IC_API_KEY"))))
	base64.StdEncoding.Encode(encodedAPIKey, []byte(os.Getenv("IC_API_KEY")))

	credentialsData := fmt.Sprintf("IBMCLOUD_%s=%s\nIBMCLOUD_%s=%s", core.PROPNAME_AUTH_TYPE, core.AUTHTYPE_IAM, core.PROPNAME_APIKEY, encodedAPIKey)
	// If there is an endpoint override for IAM, we must inject it into the credentials data
	if len(platform.ServiceEndpoints) > 0 {
		for _, endpoint := range platform.ServiceEndpoints {
			if endpoint.Name == configv1.IBMCloudServiceIAM {
				credentialsData = fmt.Sprintf("%s\nIBMCLOUD_%s=%s", core.PROPNAME_AUTH_URL, credentialsData, endpoint.URL)
				break
			}
		}
	}
	ibmcloudCreds.Data[core.DEFAULT_CREDENTIAL_FILE_NAME] = []byte(credentialsData)

	manifests = append(manifests, &asset.RuntimeFile{
		Object: ibmcloudCreds,
		File:   asset.File{Filename: "01_ibmcloud-creds.yaml"},
	})

	// Collect and build information for Cluster manifest
	resourceGroup := clusterID.InfraID
	if platform.ResourceGroupName != "" {
		resourceGroup = platform.ResourceGroupName
	}
	networkResourceGroup := resourceGroup
	if platform.NetworkResourceGroupName != "" {
		networkResourceGroup = platform.NetworkResourceGroupName
	}
	vpcName := platform.GetVPCName()
	if vpcName == "" {
		vpcName = fmt.Sprintf("%s-vpc", clusterID.InfraID)
	}

	// Create the ImageSpec details, generating the names of the COS resources we expect to use.
	// They should be created by PreProvision step.
	cosInstanceNamePtr := ptr.To(fmt.Sprintf("%s-cos", clusterID.InfraID))
	cosBucketNamePtr := ptr.To(fmt.Sprintf("%s-vsi-image", clusterID.InfraID))
	// Trim the gzip extension (and anything else) from the imageName
	trimmedImageName := strings.SplitN(imageName, ".gz", 2)[0]
	imageSpec := &capibmcloud.ImageSpec{
		Name:            fmt.Sprintf("%s-rhcos", clusterID.InfraID),
		COSInstance:     cosInstanceNamePtr,
		COSBucket:       cosBucketNamePtr,
		COSBucketRegion: ptr.To(platform.Region),
		COSObject:       ptr.To(trimmedImageName),
		OperatingSystem: ptr.To(operatingSystem),
		ResourceGroup:   &capibmcloud.GenericResourceReference{
			Name: ptr.To(resourceGroup),
		},
	}

	// Get and transform Subnets into CAPI.Subnets
	controlPlaneSubnets, err := metadata.ControlPlaneSubnets(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed collecting control plane subnets %w", err)
	}
	// If no Control Plane subnets were provided in InstallConfig, we build a default set to cover all zones in the region.
	// TODO(cjschaef): We may need to get the list of AZ's from the InstallConfig.ControlPlane.Platform.IBMCloud.Zones info.
	if len(controlPlaneSubnets) == 0 {
		zones, err := client.GetVPCZonesForRegion(context.TODO(), platform.Region)
		if err != nil {
			return nil, fmt.Errorf("failed collecting zones in region: %w", err)
		}
		if controlPlaneSubnets == nil {
			controlPlaneSubnets = make(map[string]ibmcloudic.Subnet, 0)
		}
		for _, zone := range zones {
			subnetName, err := ibmcloudic.CreateSubnetName(clusterID.InfraID, "master", zone)
			if err != nil {
				return nil, fmt.Errorf("failed creating subnet name: %w", err)
			}
			// Typically, the map is keyed by the Subnet ID, but we don't have that if we are generating new subnet names. Since the ID's don't get used in Cluster manifest generation, we should be okay, as the key is ignored during ibmcloudic.Subnet to capibmcloud.Subnet transition.
			controlPlaneSubnets[subnetName] = ibmcloudic.Subnet{
				Name: subnetName,
				Zone: zone,
			}
		}
	}
	capiControlPlaneSubnets := getCAPISubnets(controlPlaneSubnets)

	computeSubnets, err := metadata.ComputeSubnets(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed collecting compute subnets %w", err)
	}
	// If no Compute subnets were provided in InstallConfig, we build a default set to cover all zones in the region.
	// NOTE(cjschaef): We may need to get the list of AZ's from the InstallConfig.Compute.Platform.IBMCloud.Zones info.
	if len(computeSubnets) == 0 {
		zones, err := client.GetVPCZonesForRegion(context.TODO(), platform.Region)
		if err != nil {
			return nil, fmt.Errorf("failed collecting zones in region: %w", err)
		}
		if computeSubnets == nil {
			computeSubnets = make(map[string]ibmcloudic.Subnet, 0)
		}
		for _, zone := range zones {
			subnetName, err := ibmcloudic.CreateSubnetName(clusterID.InfraID, "worker", zone)
			if err != nil {
				return nil, fmt.Errorf("failed creating subnet name: %w", err)
			}
			// Typically, the map is keyed by the Subnet ID, but we don't have that if we are generating new subnet names. Since the ID's don't get used in Cluster manifest generation, we should be okay, as the key is ignored during ibmcloudic.Subnet to capibmcloud.Subnet transition.
			computeSubnets[subnetName] = ibmcloudic.Subnet{
				Name: subnetName,
				Zone: zone,
			}
		}
	}
	capiComputeSubnets := getCAPISubnets(computeSubnets)

	// Create a consolidated set of all subnets, to use when generating SecurityGroups (this should prevent duplicates that appear in both subnet slices), resulting in duplicate SecurityGroupRules for subnet CIDR's. We may not have CIDR's until Infrastructure creation, so rely on Subnet names, to lookup CIDR's at runtime.
	capiConsolidatedSubnets := consolidateCAPISubnets(capiControlPlaneSubnets, capiComputeSubnets)
	vpcSecurityGroups := getVPCSecurityGroups(clusterID.InfraID, vpcName, networkResourceGroup, capiConsolidatedSubnets)

	// Get the LB's
	loadBalancers := getLoadBalancers(clusterID.InfraID, installConfig.Config.Publish)

	// Create the IBMVPCCluster manifest
	ibmcloudCluster := &capibmcloud.IBMVPCCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: capibmcloud.GroupVersion.String(),
			Kind:       "IBMVPCCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID.InfraID,
			Namespace: capiutils.Namespace,
			Annotations: map[string]string{
				"vpc.cluster.x-k8s.io/create-infra": "true",
			},
		},
		Spec: capibmcloud.IBMVPCClusterSpec{
			ControlPlaneEndpoint: capi.APIEndpoint{
				Host: fmt.Sprintf("api.%s.%s", installConfig.Config.ObjectMeta.Name, installConfig.Config.BaseDomain),
				Port: 6443,
			},
			Image:         imageSpec,
			LoadBalancers: loadBalancers,
			NetworkSpec: &capibmcloud.VPCNetworkSpec{
				ResourceGroup:           ptr.To(networkResourceGroup),
				SecurityGroups:          vpcSecurityGroups,
				ComputeSubnetsSpec:      capiComputeSubnets,
				ControlPlaneSubnetsSpec: capiControlPlaneSubnets,
				VPC: &capibmcloud.VPCResource{
					Name: ptr.To(vpcName),
				},
			},
			Region:        platform.Region,
			ResourceGroup: resourceGroup,
		},
	}

	manifests = append(manifests, &asset.RuntimeFile{
		Object: ibmcloudCluster,
		File:   asset.File{Filename: "01_ibmcloud-cluster.yaml"},
	})

	return &capiutils.GenerateClusterAssetsOutput{
		Manifests: manifests,
		InfrastructureRef: &corev1.ObjectReference{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
			Kind:       "IBMVPCCluster",
			Name:       ibmcloudCluster.Name,
			Namespace:  ibmcloudCluster.Namespace,
		},
	}, nil
}

// consolidateCAPISubnets will attempt to consolidate two Subnet slices, and attempt to remove any duplicated Subnets (appear in both slices).
// This does not attempt to remove duplicate Subnets that exist in a single slice however.
func consolidateCAPISubnets(subnetsA []capibmcloud.Subnet, subnetsB []capibmcloud.Subnet) []capibmcloud.Subnet {
	consolidatedSubnets := make([]capibmcloud.Subnet, len(subnetsA))
	copiedSubnetNames := make(map[string]bool, 0)

	for index, subnet := range subnetsA {
		consolidatedSubnets[index] = subnet
		copiedSubnetNames[*subnet.Name] = true
	}

	for _, subnet := range subnetsB {
		// If we don't already have the Subnet from subnetsA, append it to the consolidated list
		if _, okay := copiedSubnetNames[*subnet.Name]; !okay {
			consolidatedSubnets = append(consolidatedSubnets, subnet)
		}
	}
	return consolidatedSubnets
}

// getCAPISubnets converts InstallConfig based Subnets to CAPI based Subnets for Cluster manifest generation.
func getCAPISubnets(subnets map[string]ibmcloudic.Subnet) []capibmcloud.Subnet {
	subnetList := make([]capibmcloud.Subnet, 0)
	for _, subnet := range subnets {
		subnetList = append(subnetList, capibmcloud.Subnet{
			Name: ptr.To(subnet.Name),
			Zone: ptr.To(subnet.Zone),
		})
	}
	return subnetList
}

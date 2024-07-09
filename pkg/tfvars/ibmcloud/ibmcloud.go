package ibmcloud

import (
	"encoding/json"
	"fmt"

	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/installer/pkg/rhcos/cache"
	"github.com/openshift/installer/pkg/types"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
	ibmcloudprovider "github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1"
)

// IBMCloudEndpointJSONFileName is the file containing the IBM Cloud Terraform provider's endpoint override JSON.
const IBMCloudEndpointJSONFileName = "ibmcloud_endpoints_override.json"

// Auth is the collection of credentials that will be used by terrform.
type Auth struct {
	APIKey string `json:"ibmcloud_api_key,omitempty"`
}

// DedicatedHost is the format used by terraform.
type DedicatedHost struct {
	ID      string `json:"id,omitempty"`
	Profile string `json:"profile,omitempty"`
}

type config struct {
	Auth                       `json:",inline"`
	BootstrapInstanceType      string          `json:"ibmcloud_bootstrap_instance_type,omitempty"`
	CISInstanceCRN             string          `json:"ibmcloud_cis_crn,omitempty"`
	ComputeSubnets             []string        `json:"ibmcloud_compute_subnets,omitempty"`
	ControlPlaneBootVolumeKey  string          `json:"ibmcloud_control_plane_boot_volume_key"`
	ControlPlaneSubnets        []string        `json:"ibmcloud_control_plane_subnets,omitempty"`
	DNSInstanceID              string          `json:"ibmcloud_dns_id,omitempty"`
	EndpointsJSONFile          string          `json:"ibmcloud_endpoints_json_file,omitempty"`
	ExtraTags                  []string        `json:"ibmcloud_extra_tags,omitempty"`
	ImageFilePath              string          `json:"ibmcloud_image_filepath,omitempty"`
	MasterAvailabilityZones    []string        `json:"ibmcloud_master_availability_zones"`
	MasterInstanceType         string          `json:"ibmcloud_master_instance_type,omitempty"`
	MasterDedicatedHosts       []DedicatedHost `json:"ibmcloud_master_dedicated_hosts,omitempty"`
	NetworkResourceGroupName   string          `json:"ibmcloud_network_resource_group_name,omitempty"`
	PreexistingImage           bool            `json:"ibmcloud_preexisting_image,omitempty"`
	PreexistingVPC             bool            `json:"ibmcloud_preexisting_vpc,omitempty"`
	PublishStrategy            string          `json:"ibmcloud_publish_strategy,omitempty"`
	Region                     string          `json:"ibmcloud_region,omitempty"`
	ResourceGroupName          string          `json:"ibmcloud_resource_group_name,omitempty"`
	TerraformPrivateVisibility bool            `json:"ibmcloud_terraform_private_visibility,omitempty"`
	VPC                        string          `json:"ibmcloud_vpc,omitempty"`
	VPCImageID                 string          `json:"ibmcloud_vpc_image_id,omitempty"`
	VPCImageOfferingCRN        string          `json:"ibmcloud_vpc_image_offering_crn,omitempty"`
	VPCPermitted               bool            `json:"ibmcloud_vpc_permitted,omitempty"`
	WorkerAvailabilityZones    []string        `json:"ibmcloud_worker_availability_zones"`
	WorkerDedicatedHosts       []DedicatedHost `json:"ibmcloud_worker_dedicated_hosts,omitempty"`
}

// TFVarsSources contains the parameters to be converted into Terraform variables
type TFVarsSources struct {
	Auth                       Auth
	CISInstanceCRN             string
	DNSInstanceID              string
	EndpointsJSONFile          string
	ImageURL                   string
	MasterConfigs              []*ibmcloudprovider.IBMCloudMachineProviderSpec
	MasterDedicatedHosts       []DedicatedHost
	NetworkResourceGroupName   string
	PreexistingImage           bool
	PreexistingVPC             bool
	PublishStrategy            types.PublishingStrategy
	ResourceGroupName          string
	TerraformPrivateVisibility bool
	VPCPermitted               bool
	WorkerConfigs              []*ibmcloudprovider.IBMCloudMachineProviderSpec
	WorkerDedicatedHosts       []DedicatedHost
}

// TFVars generates ibmcloud-specific Terraform variables launching the cluster.
func TFVars(sources TFVarsSources) ([]byte, error) {
	masterConfig := sources.MasterConfigs[0]
	masterAvailabilityZones := make([]string, len(sources.MasterConfigs))
	for i, c := range sources.MasterConfigs {
		masterAvailabilityZones[i] = c.Zone
	}
	workerAvailabilityZones := make([]string, len(sources.WorkerConfigs))
	for i, c := range sources.WorkerConfigs {
		workerAvailabilityZones[i] = c.Zone
	}

	// If using a pre-existing VPC Image, pull it from the masterConfig to pass to TF.
	// We expect to see either an existing VPC Image ID, or a VPC Catalog Offering CRN.
	// We differentiate by parsing the Image as a CRN (Offering CRN versus Image ID).
	var vpcImageID, vpcImageOfferingCRN, cachedImage string
	if sources.PreexistingImage && masterConfig.Image != "" {
		// We parse the Image for a CRN, if that fails we expect it is an Image ID.
		if crn, err := crn.Parse(masterConfig.Image); err != nil {
			vpcImageID = masterConfig.Image
		} else {
			// Depending on the CRN's Service Instance, we can use a Catalog Offering or VPC Custom Image.
			if crn.ServiceName == "globalcatalog-collection" && crn.ResourceType == "offering" {
				vpcImageOfferingCRN = masterConfig.Image
			} else if crn.ServiceName == "is" && crn.ResourceType == "image" {
				// Parse the Image Id from the CRN, as we use the ID only.
				vpcImageID = crn.Resource
			} else {
				return nil, fmt.Errorf("unknown service instance crn provided for image: %s", masterConfig.Image)
			}
		}
	} else if sources.PreexistingImage {
		// If an existing image was flagged for use, but no image was provided, return an error
		return nil, fmt.Errorf("failed building tfvars, expected to find an existing image for control plane machines")
	} else {
		var err error
		// Only attempt to download and use the cached image if an existing image was not provided.
		cachedImage, err = cache.DownloadImageFile(sources.ImageURL, cache.InstallerApplicationName)
		if err != nil {
			return nil, fmt.Errorf("failed to use cached ibmcloud image: %w", err)
		}
	}

	// Set pre-existing network config
	var vpc string
	masterSubnets := make([]string, len(sources.MasterConfigs))
	workerSubnets := make([]string, len(sources.WorkerConfigs))
	if sources.PreexistingVPC {
		vpc = sources.MasterConfigs[0].VPC
		for index, config := range sources.MasterConfigs {
			masterSubnets[index] = config.PrimaryNetworkInterface.Subnet
		}
		for index, config := range sources.WorkerConfigs {
			workerSubnets[index] = config.PrimaryNetworkInterface.Subnet
		}
	}

	cfg := &config{
		Auth:                       sources.Auth,
		BootstrapInstanceType:      masterConfig.Profile,
		CISInstanceCRN:             sources.CISInstanceCRN,
		ComputeSubnets:             workerSubnets,
		ControlPlaneBootVolumeKey:  masterConfig.BootVolume.EncryptionKey,
		ControlPlaneSubnets:        masterSubnets,
		DNSInstanceID:              sources.DNSInstanceID,
		EndpointsJSONFile:          sources.EndpointsJSONFile,
		ImageFilePath:              cachedImage,
		MasterAvailabilityZones:    masterAvailabilityZones,
		MasterDedicatedHosts:       sources.MasterDedicatedHosts,
		MasterInstanceType:         masterConfig.Profile,
		NetworkResourceGroupName:   sources.NetworkResourceGroupName,
		PreexistingImage:           sources.PreexistingImage,
		PreexistingVPC:             sources.PreexistingVPC,
		PublishStrategy:            string(sources.PublishStrategy),
		Region:                     masterConfig.Region,
		ResourceGroupName:          sources.ResourceGroupName,
		TerraformPrivateVisibility: sources.TerraformPrivateVisibility,
		VPC:                        vpc,
		VPCImageID:                 vpcImageID,
		VPCImageOfferingCRN:        vpcImageOfferingCRN,
		VPCPermitted:               sources.VPCPermitted,
		WorkerAvailabilityZones:    workerAvailabilityZones,
		WorkerDedicatedHosts:       sources.WorkerDedicatedHosts,

		// TODO: IBM: Future support
		// ExtraTags:               masterConfig.Tags,
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// CreateEndpointJSON creates JSON data containing IBM Cloud service endpoint override mappings.
func CreateEndpointJSON(endpoints []configv1.IBMCloudServiceEndpoint, region string) ([]byte, error) {
	// If no endpoint overrides, simply return
	if len(endpoints) == 0 {
		return nil, nil
	}

	endpointContents := ibmcloudtypes.EndpointsJSON{}
	for _, endpoint := range endpoints {
		switch endpoint.Name {
		// COS endpoint is not used in Terraform
		case configv1.IBMCloudServiceCOS:
			continue
		case configv1.IBMCloudServiceCIS:
			endpointContents.IBMCloudEndpointCIS = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceDNSServices:
			endpointContents.IBMCloudEndpointDNSServices = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceGlobalSearch:
			endpointContents.IBMCloudEndpointGlobalSearch = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceGlobalTagging:
			endpointContents.IBMCloudEndpointGlobalTagging = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceHyperProtect:
			endpointContents.IBMCloudEndpointHyperProtect = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceIAM:
			endpointContents.IBMCloudEndpointIAM = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceKeyProtect:
			endpointContents.IBMCloudEndpointKeyProtect = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceResourceController:
			endpointContents.IBMCloudEndpointResourceController = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceResourceManager:
			endpointContents.IBMCloudEndpointResourceManager = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		case configv1.IBMCloudServiceVPC:
			endpointContents.IBMCloudEndpointVPC = &ibmcloudtypes.EndpointsVisibility{
				Private: map[string]string{
					region: endpoint.URL,
				},
				Public: map[string]string{
					region: endpoint.URL,
				},
			}
		default:
			return nil, fmt.Errorf("unable to build override values for unknown service: %s", endpoint.Name)
		}
	}
	jsonData, err := json.Marshal(endpointContents)
	if err != nil {
		return nil, fmt.Errorf("failure building service endpoint override JSON data: %w", err)
	}

	// If the JSON contains no data, none was populated (jsonData == "{}"), but endpoints is not empty, we assume the Services are not required for Terraform (e.g., COS)
	// Log this as a warning, but continue as if no service endpoints were provided
	if len(jsonData) <= 2 {
		logrus.Warnf("no terraform endpoint json was created for services: %s", endpoints)
		return nil, nil
	}
	return jsonData, nil
}

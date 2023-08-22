package ibmcloud

import (
	"strings"

	configv1 "github.com/openshift/api/config/v1"
)

const (
	// IBMCloudServiceCIS is the lowercase name representation for IBM Cloud CIS
	IBMCloudServiceCIS    string = "cis"
	IBMCloudServiceCISVar string = "IBMCLOUD_CIS_API_ENDPOINT"

	// IBMCloudServiceCOS is the lowercase name representation for IBM Cloud COS
	IBMCloudServiceCOS    string = "cos"
	IBMCloudServiceCOSVar string = "IBMCLOUD_COS_CONFIG_ENDPOINT"

	// IBMCloudServiceDNSServices is the lowercase name representation for IBM Cloud DNS Services
	IBMCloudServiceDNSServices    string = "dnsservices"
	IBMCloudServiceDNSServicesVar string = "IBMCLOUD_PRIVATE_DNS_API_ENDPOINT"

	// IBMCloudServiceIAM is the lowercase name representation for IBM Cloud IAM
	IBMCloudServiceIAM    string = "iam"
	IBMCloudServiceIAMVar string = "IBMCLOUD_IAM_API_ENDPOINT"

	// IBMCloud ServiceResourceController is the lowercase name representation for IBM Cloud Resource Controller
	IBMCloudServiceResourceController    string = "resourcecontroller"
	IBMCloudServiceResourceControllerVar string = "IBMCLOUD_RESOURCE_CONTROLLER_API_ENDPOINT"

	// IBMCloudServiceResourceManager is the lowercase name representation for IBM Cloud Resource Management
	IBMCloudServiceResourceManager    string = "resourcemanager"
	IBMCloudServiceResourceManagerVar string = "IBMCLOUD_RESOURCE_MANAGEMENT_API_ENDPOINT"

	// IBMCloudServiceVPC is the lowercase name representation for IBM Cloud VPC
	IBMCloudServiceVPC    string = "vpc"
	IBMCloudServiceVPCVar string = "IBMCLOUD_IS_NG_API_ENDPOINT"
)

var (
	// IBMCloudServiceOverrides is a set of IBM Cloud services allowed to have their endpoints overridden mapped to their override environment variable
	IBMCloudServiceOverrides = map[string]string{
		IBMCloudServiceCIS:                IBMCloudServiceCISVar,
		IBMCloudServiceCOS:                IBMCloudServiceCOSVar,
		IBMCloudServiceDNSServices:        IBMCloudServiceDNSServicesVar,
		IBMCloudServiceIAM:                IBMCloudServiceIAMVar,
		IBMCloudServiceResourceController: IBMCloudServiceResourceControllerVar,
		IBMCloudServiceResourceManager:    IBMCloudServiceResourceManagerVar,
		IBMCloudServiceVPC:                IBMCloudServiceVPCVar,
	}
)

// CheckServiceEndpointOverride checks whether a service has an override endpoint
func CheckServiceEndpointOverride(service string, serviceEndpoints []configv1.IBMCloudServiceEndpoint) string {
        if len(serviceEndpoints) > 0 {
                for _, endpoint := range serviceEndpoints {
                        if strings.ToLower(endpoint.Name) == service {
                                return endpoint.URL
                        }
                }
        }
        return ""
}

// Platform stores all the global configuration that all machinesets use.
type Platform struct {
	// Region specifies the IBM Cloud region where the cluster will be
	// created.
	Region string `json:"region"`

	// ResourceGroupName is the name of an already existing resource group where the
	// cluster should be installed. If empty, a new resource group will be created
	// for the cluster.
	// +optional
	ResourceGroupName string `json:"resourceGroupName,omitempty"`

	// NetworkResourceGroupName is the name of an already existing resource group
	// where an existing VPC and set of Subnets exist, to be used during cluster
	// creation.
	// +optional
	NetworkResourceGroupName string `json:"networkResourceGroupName,omitempty"`

	// VPCName is the name of an already existing VPC to be used during cluster
	// creation.
	// +optional
	VPCName string `json:"vpcName,omitempty"`

	// ControlPlaneSubnets are the names of already existing subnets where the
	// cluster control plane nodes should be created.
	// +optional
	ControlPlaneSubnets []string `json:"controlPlaneSubnets,omitempty"`

	// ComputeSubnets are the names of already existing subnets where the cluster
	// compute nodes should be created.
	// +optional
	ComputeSubnets []string `json:"computeSubnets,omitempty"`

	// DefaultMachinePlatform is the default configuration used when installing
	// on IBM Cloud for machine pools which do not define their own platform
	// configuration.
	// +optional
	DefaultMachinePlatform *MachinePool `json:"defaultMachinePlatform,omitempty"`

	// ServiceEndpoints is a list which contains custom endpoints to override default
	// service endpoints of IBM Cloud Services.
	// There must only be one ServiceEndpoint for a service (no duplicates).
	// +optional
	ServiceEndpoints []configv1.IBMCloudServiceEndpoint `json:"serviceEndpoints,omitempty"`
}

// ClusterResourceGroupName returns the name of the resource group for the cluster.
func (p *Platform) ClusterResourceGroupName(infraID string) string {
	if len(p.ResourceGroupName) > 0 {
		return p.ResourceGroupName
	}
	return infraID
}

// GetVPCName returns the user provided name of the VPC for the cluster.
func (p *Platform) GetVPCName() string {
	if len(p.VPCName) > 0 {
		return p.VPCName
	}
	return ""
}

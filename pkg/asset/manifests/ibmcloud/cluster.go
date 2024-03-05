package ibmcloud

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/IBM/go-sdk-core/v5/core"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	ibmcloudic "github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
	"github.com/openshift/installer/pkg/types"
)

// GenerateClusterAssets generates the manifests for the cluster-api.
func GenerateClusterAssets(installConfig *installconfig.InstallConfig, clusterID *installconfig.ClusterID) (*capiutils.GenerateClusterAssetsOutput, error) {
	manifests := []*asset.RuntimeFile{}
	// mainCIDR := capiutils.CIDRFromInstallConfig(installConfig)
	platform := installConfig.Config.Platform.IBMCloud

	// Create IBM Cloud Credentials for IBM Cloud CAPI
	ibmcloudCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-credentials",
			Namespace: capiutils.Namespace,
		},
		Data: make(map[string][]byte),
	}

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

	resourceGroup := clusterID.InfraID
	if platform.ResourceGroupName != "" {
		resourceGroup = platform.ResourceGroupName
	}
	networkResourceGroup := resourceGroup
	if platform.NetworkResourceGroupName != "" {
		networkResourceGroup = platform.NetworkResourceGroupName
	}

	controlPlaneSubnets := getSubnets(installConfig.IBMCloud.ComputeSubnetNames(context.TODO()))
	computeSubnets := getSubnets(installConfig.IBMCloud.ComputeSubnets(context.TODO()))
	vpcSecurityGroups, err := getVPCSecurityGroups(clusterID.InfraID, platform.GetVPCName(), networkResourceGroup, controlPlaneSubnets, computeSubnets)
	if err != nil {
		return nil, fmt.Errorf("failed building VPC Security Groups %w", err)
	}

	ibmcloudCluster := &capibmcloud.IBMVPCCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID.InfraID,
			Namespace: capiutils.Namespace,
		},
		Spec: capibmcloud.IBMVPCClusterSpec{
			ControlPlaneEndpoint: capi.APIEndpoint{
				Host: fmt.Sprintf("api.%s.%s", &installConfig.Config.ObjectMeta.Name, installConfig.Config.BaseDomain),
			},
			COSInstance: cosInstance,
			NetworkSpec: capibmcloud.IBMVPCNetworkSpec{
				ResourceGroup: networkResourceGroup,
				SecurityGroups: vpcSecurityGroups,
				ComputeSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: computeSubnets,
				},
				ControlPlaneSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: controlPlaneSubnets,
				},
				VPC: &capibmcloud.IBMVPCResourceReference{
					Name: platform.GetVPCName(),
				},
			},
			Region:        platform.Region,
			ResourceGroup: resourceGroup,
		},
	}

	if installConfig.Config.Publish == types.InternalPublishingStrategy {
		dnsInstance, err := getDNSServicesInstance(installConfig.Config.BaseDomain)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		ibmcloudCluster.DNSServicesInstance = dnsInstance
	} else {
		cisInstance, err := getCISInstance(installConfig.Config.BaseDomain)
		if err != nil {
			return nil, fmt.Errorf("%w", err)
		}
		ibmcloudCluster.CISInstance = cisInstance
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

func getCISInstance(domain string) (*capibmcloud.CISInstance, error) {
	// TODO(cjschaef): Complete CIS lookup
	return &capibmcloud.CISInstance{
		Domain: "cis-base-domain",
		Name:   "cis-instance-name",
	}, nil
}

func getDNSServicesInstance(domain string) (*capibmcloud.DNSServicesInstance, error) {
	// TODO(cjschaef): Complete DNS Services lookup
	return &capibmcloud.DNSServicesInstance{
		Name: "dns-instance-name",
		Zone: "dns-instance-zone",
	}, nil
}

func getSubnets(subnets map[string]ibmcloudic.Subnet) []capibmcloud.Subnet {
	subnetList := make([]capibmcloud.Subnet, 0, len(subnets))
	for _, subnet := range subnets {
		subnetList = append(subnetList, capibmcloud.Subnet{
			Name: ptr.To(subnet.Name),
			Zone: ptr.To(subnet.Zone),
		})
	}
	return subnetList
}

func getVPCSecurityGroups(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) ([]capibmcloud.SecurityGroup, error) {
	clusterWideSGName := fmt.Sprintf("%s-sg-cluster-wide", infraID)
	openshiftNetSGName := fmt.Sprintf("%s-sg-openshift-net", infraID)
	kubeAPILBSGName := fmt.Sprintf("%s-sg-kube-api-lb", infraID)
	controlPlaneSGName := fmt.Sprintf("%s-sg-control-plane", infraID)
	cpInternalSGName := fmt.Sprintf("%s-sg-cp-internal", infraID)

	// Build sets of Remotes for Security Group Rules
	// - cluster-wide SSH rule (for CP and Compute subnets)
	clusterWideSSHRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
	// - openshift-net TCP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortTCPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
	// - openshift-net UDP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortUDPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
	for index, subnet := range allSubnets {
		clusterWideSSHRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
		openshiftNetworkNodePortTCPRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
		openshiftNetworkNodePortUDPRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
	}

	return []capibmcloud.SecurityGroup{
		{
			Name: clusterWideSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemoteSpec{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 22,
							MinimumPort: 22,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes:  clusterWideSSHRemotes,
					},
				},
				{
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemoteSpec{
						Protocol: capibmcloud.SecurityGroupRuleProtocolICMP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: clusterWideSGName,
							},
						},
					},
				},
				{
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemoteSpec{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 4789,
							MinimumPort: 4789,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: clusterWideSGName,
							},
						},
					},
				},
				{
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemoteSpec{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 6081,
							MinimumPort: 6081,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: clusterWideSGName,
							},
						},
					},
				},
				{
					Action: capibmcloud.SecurityGroupRuleActionAllow,
					Destination: &capibmcloud.SecurityGroupRuleRemoteSpec{
						Protocol: capibmcloudSecurityGroupRuleProtocolAny,
						Remote: capibmcloudSecurityGroupRuleRemote{
							RemoteType: capibmcloud.SecurityGroupRuleRemoteTypeAny,
						},
					},
					Direction: capibmcloud.SecurityGroupRuleDirectionOutbound,
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			Name: openshiftNetSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					Name:
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			Name: kubeAPILBSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					Name:
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			Name: controlPlaneSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					Name:
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			Name: cpInternalSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					Name:
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
	}, nil
}

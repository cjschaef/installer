package ibmcloud

import (
	"context"
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
)

// GenerateClusterAssets generates the manifests for the cluster-api.
func GenerateClusterAssets(installConfig *installconfig.InstallConfig, clusterID *installconfig.ClusterID, cosID string) (*capiutils.GenerateClusterAssetsOutput, error) {
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

	cosInstance := &capibmcloud.COSInstanceReference{
		ID: cosID,
	}

	controlPlaneSubnets, err := installConfig.IBMCloud.ControlPlaneSubnets(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed collecting control plane subnets %w", err)
	}
	capiControlPlaneSubnets := getSubnets(controlPlaneSubnets)
	computeSubnets, err := installConfig.IBMCloud.ComputeSubnets(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed collecting compute subnets %w", err)
	}
	capiComputeSubnets := getSubnets(computeSubnets)
	
	// Create a consolidated set of all subnets, to use when generating SecurityGroups (this should prevent duplicates that appear in both subnet slices), resulting in duplicate SecurityGroupRules for subnet CIDR's. We may not have CIDR's until Infrastructure creation, so rely on Subnet names, to lookup CIDR's at runtime.
	capiConsolidatedSubnets := consolidateSubnets(capiControlPlaneSubnets, capiComputeSubnets)
	vpcSecurityGroups, err := getVPCSecurityGroups(clusterID.InfraID, platform.GetVPCName(), networkResourceGroup, capiConsolidatedSubnets)
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
			NetworkSpec: &capibmcloud.VPCNetworkSpec{
				ResourceGroup: networkResourceGroup,
				SecurityGroups: vpcSecurityGroups,
				ComputeSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: capiComputeSubnets,
				},
				ControlPlaneSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: capiControlPlaneSubnets,
				},
				VPC: &capibmcloud.IBMVPCResourceReference{
					Name: platform.GetVPCName(),
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

// consolidateSubnets will attempt to consolidate two Subnet slices, and attempt to remove any duplicated Subnets (appear in both slices).
// This does not attempt to remove duplicate Subnets that exist in a single slice however.
func consolidateSubnets(subnetsA []capibmcloud.Subnet, subnetsB []capibmcloud.Subnet) []capibmcloud.Subnet {
	consolidatedSubnets := make([]capibmcloud.Subnet, len(subnetsA), (len(subnetsA) + len(subnetsB)))
	copiedSubnetNames := make(map[string]bool, len(subnetsA))

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

// getSubnets converts InstallConfig based Subnets to CAPI based Subnets for Cluster manifest generation.
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
	openshiftNetworkNodePortUDPRemotes := make([]capibmcloud.SecurityGroupRulRemote, 0, len(allSubnets))
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
			// cluster-wide SG definition
			Name: clusterWideSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					// SSH inbound cluster-wide
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 22,
							MinimumPort: 22,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes:  clusterWideSSHRemotes,
					},
				},
				{
					// ICMP inbound cluster-wide
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
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
					// VXLAN and Geneve - port 4789
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
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
					// VXLAN and Geneve - port 6081
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
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
					// Outbound for cluster-wide
					Action: capibmcloud.SecurityGroupRuleActionAllow,
					Destination: &capibmcloud.SecurityGroupRuleRemotePrototype{
						Protocol: capibmcloudSecurityGroupRuleProtocolAny,
						Remotes: capibmcloudSecurityGroupRuleRemote{
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
			// openshift-network SG definition
			Name: openshiftNetSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					// Host level services - TCP
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 9999,
							MinimumPort: 9000,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: openshiftNetSGName,
							},
						},
					},
				},
				{
					// Host level services - UDP
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 9999,
							MinimumPort: 9000,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: openshiftNetSGName,
							},
						},
					},
				},
				{
					// Kubernetes default ports
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 10250,
							MinimumPort: 10250,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloudSecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: openshiftNetSGName,
							},
						},
					},
				},
				{
					// IPsec IKE - port 500
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaxiumuPort: 500,
							MinimumPort: 500,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: openshiftNetSGName,
							},
						},
					},
				},
				{
					// IPsec IKE NAT-T - port 4500
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 4500,
							MinimumPort: 4500,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: openshiftNetSGName,
							},
						},
					},
				},
				{
					// Kubernetes node ports - TCP
					// Allows access to node ports from within VPC subnets to accommodate CCM LBs
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 32767,
							MinimumPort: 30000,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes:  openshiftNetworkNodePortTCPRemotes,
					},
				},
				{
					// Kubernetes node ports - UDP
					// Allows access to node ports from within VPC subnets to accommodate CCM LBs
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 32767,
							MinimumPort: 30000,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
						Remotes:  openshiftNetworkNodePortUDPRemotes,
					},
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			// kube-api-lb SG definition
			Name: kubeAPILBSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					// Kubernetes API LB - inbound
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 6443,
							MinimumPort: 6443,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType: capibmcloud.SecurityGroupRuleRemoteTypeAny,
							},
						},
					},
				},
				{
					// Kubernetes API LB - outbound
					Action: capibmcloud.SecurityGroupRuleActionAllow,
					Destination: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 6443,
							MinimumPort: 6443,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: controlPlaneSGName,
							},
						},
					},
					Direction: capibmcloud.SecurityGroupRuleDirectionOutbound,
				},
				{
					// Machine Config Server LB - inbound
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 22623,
							MinimumPort: 22623,
						},
						Protocol: capibmcloud.SecurityGropuRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: clusterWideSGName,
							},
						},
					},
				},
				{
					// Machine Config Server LB - outbound
					 Action: capibmcloud.SecurityGroupRuleActionAllow,
					 Destination: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 22623,
							MinimumPort: 22623,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloudSecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: controlPlaneSGName,
							},
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
			// control-plane SG definition
			Name: controlPlaneSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					// Kubernetes API -inbound via cluster
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 6443,
							MinimumPort: 6443,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: clusterWideSGName,
							},
						},
					},
				},
				{
					// Kubernetes API - inbound via LB
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &ibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 6443,
							MinimumPort: 6443,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: kubeAPILBSGName,
							},
						},
					},
				},
				{
					// Machine Config Server - inbound via LB
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 22623,
							MinimumPort: 22623,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: kubeAPILBSGName,
							},
						},
					},
				},
				{
					// Kubernetes default ports
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemotePrototype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 10259,
							MinimumPort: 10257,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGName,
						},
					},
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
		{
			// cp-internal SG definition
			Name: cpInternalSGName,
			ResourceGroup: resourceGroupName,
			Rules: []capibmcloud.SecurityGroupRule{
				{
					// etcd internal traffic
					Action:    capibmcloud.SecurityGroupRuleActionAllow,
					Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
					Source: &capibmcloud.SecurityGroupRuleRemoteProtottype{
						PortRange: &capibmcloud.PortRange{
							MaximumPort: 2380,
							MinimumPort: 2379,
						},
						Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
						Remotes: []capibmcloud.SecurityGroupRuleRemote{
							{
								RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
								SecurityGroupName: cpInternalSGName,
							},
						},
					},
				},
			},
			VPC: capibmcloud.VPCResourceReference{
				Name: vpcName,
			},
		},
	}, nil
}

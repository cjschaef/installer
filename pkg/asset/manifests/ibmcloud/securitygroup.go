package ibmcloud

import (
	"fmt"

	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
)

const (
	clusterWideSGNamePostfix  = "sg-cluster-wide"
	openshiftNetSGNamePostfix = "sg-openshift-net"
	kubeAPILBSGNamePostfix    = "sg-kube-api-lb"
	controlPlaneSGNamePostfix = "sg-control-plane"
	cpInternalSGNamePostfix   = "sg-cp-internal"
)

func buildClusterWideSecurityGroup(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) capibmcloud.SecurityGroup {
	clusterWideSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, clusterWideSGNamePostfix))
	vpcNamePtr := ptr.To(vpcName)
	resourceGroupNamePtr := ptr.To(resourceGroupName)

	// Build set of Remotes for Security Group Rules
	// - cluster-wide SSH rule (for CP and Compute subnets)
	clusterWideSSHRemotes := make([]capibmcloud.SecurityGroupRuleRemote, len(allSubnets))
	for index, subnet := range allSubnets {
		clusterWideSSHRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
	}

	return capibmcloud.SecurityGroup{
		Name:          clusterWideSGNamePtr,
		ResourceGroup: resourceGroupNamePtr,
		Rules: []*capibmcloud.SecurityGroupRule{
			{
				// SSH inbound cluster-wide
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
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
				Source: &capibmcloud.SecurityGroupRulePrototype{
					Protocol: capibmcloud.SecurityGroupRuleProtocolIcmp,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
			{
				// VXLAN and Geneve - port 4789
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 4789,
						MinimumPort: 4789,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
			{
				// VXLAN and Geneve - port 6081
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 6081,
						MinimumPort: 6081,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
			{
				// Outbound for cluster-wide
				Action: capibmcloud.SecurityGroupRuleActionAllow,
				Destination: &capibmcloud.SecurityGroupRulePrototype{
					Protocol: capibmcloud.SecurityGroupRuleProtocolAll,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType: capibmcloud.SecurityGroupRuleRemoteTypeAny,
						},
					},
				},
				Direction: capibmcloud.SecurityGroupRuleDirectionOutbound,
			},
		},
		VPC: &capibmcloud.VPCResourceReference{
			Name: vpcNamePtr,
		},
	}
}

func buildOpenshiftNetSecurityGroup(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) capibmcloud.SecurityGroup {
	openshiftNetSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, openshiftNetSGNamePostfix))
	vpcNamePtr := ptr.To(vpcName)
	resourceGroupNamePtr := ptr.To(resourceGroupName)

	// Build sets of Remotes for Security Group Rules
	// - openshift-net TCP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortTCPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, len(allSubnets))
	// - openshift-net UDP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortUDPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, len(allSubnets))
	for index, subnet := range allSubnets {
		openshiftNetworkNodePortTCPRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
		openshiftNetworkNodePortUDPRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
	}

	return capibmcloud.SecurityGroup{
		Name:          openshiftNetSGNamePtr,
		ResourceGroup: resourceGroupNamePtr,
		Rules: []*capibmcloud.SecurityGroupRule{
			{
				// Host level services - TCP
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 9999,
						MinimumPort: 9000,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: openshiftNetSGNamePtr,
						},
					},
				},
			},
			{
				// Host level services - UDP
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 9999,
						MinimumPort: 9000,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: openshiftNetSGNamePtr,
						},
					},
				},
			},
			{
				// Kubernetes default ports
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 10250,
						MinimumPort: 10250,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: openshiftNetSGNamePtr,
						},
					},
				},
			},
			{
				// IPsec IKE - port 500
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 500,
						MinimumPort: 500,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: openshiftNetSGNamePtr,
						},
					},
				},
			},
			{
				// IPsec IKE NAT-T - port 4500
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 4500,
						MinimumPort: 4500,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: openshiftNetSGNamePtr,
						},
					},
				},
			},
			{
				// Kubernetes node ports - TCP
				// Allows access to node ports from within VPC subnets to accomodate CCM LBs
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
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
				// Allows access to node ports from within VPC subnets to accomodate CCM LBs
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 32767,
						MinimumPort: 30000,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolUDP,
					Remotes:  openshiftNetworkNodePortUDPRemotes,
				},
			},
		},
		VPC: &capibmcloud.VPCResourceReference{
			Name: vpcNamePtr,
		},
	}
}

func buildKubeAPILBSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	kubeAPILBSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, kubeAPILBSGNamePostfix))
	controlPlaneSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, controlPlaneSGNamePostfix))
	clusterWideSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, clusterWideSGNamePostfix))
	vpcNamePtr := ptr.To(vpcName)
	resourceGroupNamePtr := ptr.To(resourceGroupName)

	return capibmcloud.SecurityGroup{
		Name:          kubeAPILBSGNamePtr,
		ResourceGroup: resourceGroupNamePtr,
		Rules: []*capibmcloud.SecurityGroupRule{
			{
				// Kubernetes API LB - inbound
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
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
				Destination: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 6443,
						MinimumPort: 6443,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: controlPlaneSGNamePtr,
						},
					},
				},
				Direction: capibmcloud.SecurityGroupRuleDirectionOutbound,
			},
			{
				// Machine Config Server LB - inbound
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 22623,
						MinimumPort: 22623,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
			{
				// Machine Config Server LB - outbound
				Action: capibmcloud.SecurityGroupRuleActionAllow,
				Destination: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 22623,
						MinimumPort: 22623,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: controlPlaneSGNamePtr,
						},
					},
				},
				Direction: capibmcloud.SecurityGroupRuleDirectionOutbound,
			},
		},
		VPC: &capibmcloud.VPCResourceReference{
			Name: vpcNamePtr,
		},
	}
}

func buildControlPlaneSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	controlPlaneSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, controlPlaneSGNamePostfix))
	clusterWideSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, clusterWideSGNamePostfix))
	kubeAPILBSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, kubeAPILBSGNamePostfix))
	vpcNamePtr := ptr.To(vpcName)
	resourceGroupNamePtr := ptr.To(resourceGroupName)

	return capibmcloud.SecurityGroup{
		Name:          controlPlaneSGNamePtr,
		ResourceGroup: resourceGroupNamePtr,
		Rules: []*capibmcloud.SecurityGroupRule{
			{
				// Kubernetes API - inbound via cluster
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 6443,
						MinimumPort: 6443,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
			{
				// Kubernetes API - inbound via LB
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 6443,
						MinimumPort: 6443,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: kubeAPILBSGNamePtr,
						},
					},
				},
			},
			{
				// Machine Config Server - inbound via LB
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 22623,
						MinimumPort: 22623,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: kubeAPILBSGNamePtr,
						},
					},
				},
			},
			{
				// Kubernetes default ports
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 10259,
						MinimumPort: 10257,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: clusterWideSGNamePtr,
						},
					},
				},
			},
		},
		VPC: &capibmcloud.VPCResourceReference{
			Name: vpcNamePtr,
		},
	}
}

func buildCPInternalSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	cpInternalSGNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, cpInternalSGNamePostfix))
	vpcNamePtr := ptr.To(vpcName)
	resourceGroupNamePtr := ptr.To(resourceGroupName)

	return capibmcloud.SecurityGroup{
		Name:          cpInternalSGNamePtr,
		ResourceGroup: resourceGroupNamePtr,
		Rules: []*capibmcloud.SecurityGroupRule{
			{
				// etcd internal traffic
				Action:    capibmcloud.SecurityGroupRuleActionAllow,
				Direction: capibmcloud.SecurityGroupRuleDirectionInbound,
				Source: &capibmcloud.SecurityGroupRulePrototype{
					PortRange: &capibmcloud.PortRange{
						MaximumPort: 2380,
						MinimumPort: 2379,
					},
					Protocol: capibmcloud.SecurityGroupRuleProtocolTCP,
					Remotes: []capibmcloud.SecurityGroupRuleRemote{
						{
							RemoteType:        capibmcloud.SecurityGroupRuleRemoteTypeSG,
							SecurityGroupName: cpInternalSGNamePtr,
						},
					},
				},
			},
		},
		VPC: &capibmcloud.VPCResourceReference{
			Name: vpcNamePtr,
		},
	}
}

func getVPCSecurityGroups(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) []capibmcloud.SecurityGroup {
	// IBM Cloud currently relies on 5 SecurityGroups to manage traffic
	securityGroups := make([]capibmcloud.SecurityGroup, 0, 5)
	securityGroups = append(securityGroups, buildClusterWideSecurityGroup(infraID, vpcName, resourceGroupName, allSubnets))
	securityGroups = append(securityGroups, buildOpenshiftNetSecurityGroup(infraID, vpcName, resourceGroupName, allSubnets))
	securityGroups = append(securityGroups, buildKubeAPILBSecurityGroup(infraID, vpcName, resourceGroupName))
	securityGroups = append(securityGroups, buildControlPlaneSecurityGroup(infraID, vpcName, resourceGroupName))
	securityGroups = append(securityGroups, buildCPInternalSecurityGroup(infraID, vpcName, resourceGroupName))
	return securityGroups
}

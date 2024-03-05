package ibmcloud


func buildClusterWideSecurityGroup(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) capibmcloud.SecurityGroup {
	sgName := fmt.Sprintf("%s-sg-cluster-wide", infraID)

	// Build set of Remotes for Security Group Rules
	// - cluster-wide SSH rule (for CP and Compute subnets)
	clusterWideSSHRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
	for index, subnet := range allSubnets {
		clusterWideSSHRemotes[index] = capibmcloud.SecurityGroupRuleRemote{
			RemoteType:     capibmcloud.SecurityGroupRuleRemoteTypeCIDR,
			CIDRSubnetName: subnet.Name,
		}
	}

	return capibmcloud.SecurityGroup{
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
	}
}

func buildOpenshiftNetSecurityGroup(infraID string, vpcName string, resourceGroupName string, allSubnets []capibmcloud.Subnet) capibmcloud.SecurityGroup {
	openshiftNetSGName := fmt.Sprintf("%s-sg-openshift-net", infraID)

	// Build sets of Remotes for Security Group Rules
	// - openshift-net TCP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortTCPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
	// - openshift-net UDP rule for Node Ports (for CP and Compute subnets)
	openshiftNetworkNodePortUDPRemotes := make([]capibmcloud.SecurityGroupRuleRemote, 0, len(allSubnets))
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
	}
}

func buildKubeAPILBSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	kubeAPILBSGName := fmt.Sprintf("%s-sg-kube-api-lb", infraID)
	return capibmcloud.SecurityGroup{
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
	}
}

func buildControlPlaneSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	controlPlaneSGName := fmt.Sprintf("%s-sg-control-plane", infraID)
	return capibmcloud.SecurityGroup{
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
	}
}

func buildCPInternalSecurityGroup(infraID string, vpcName string, resourceGroupName string) capibmcloud.SecurityGroup {
	cpInternalSGName := fmt.Sprintf("%s-sg-cp-internal", infraID)
	return capibmcloud.SecurityGroup{
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
	return securityGroups, nil
}

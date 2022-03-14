package ibmcloud

// Platform stores all the global configuration that all machinesets use.
type Platform struct {
	// Region specifies the IBM Cloud region where the cluster will be
	// created.
	Region string `json:"region"`

	// ResourceGroupName is the name of an already existing resource group where the
	// cluster should be installed. This resource group should only be used for
	// this specific cluster and the cluster components will assume ownership of
	// all resources in the resource group.
	//
	// If empty, a new resource group will be created for the cluster
	// +optional
	ResourceGroupName string `json:"resourceGroupName,omitempty"`

	// VPC is the name of an already existing VPC where the cluster
	// should be installed
	// If empty, a new VPC will be created
	// +optional
	VPC string `json:"vpc,omitempty"`

	// Subnets is the set of an already existing subnets where
	// the cluster's nodes should be deployed on (requires existing VPC)
	// If empty, new subnets will be created
	// +optional
	Subnets []string `json:"subnets,omitempty"`

	// DefaultMachinePlatform is the default configuration used when installing
	// on IBM Cloud for machine pools which do not define their own platform
	// configuration.
	// +optional
	DefaultMachinePlatform *MachinePool `json:"defaultMachinePlatform,omitempty"`
}

// ClusterResourceGroupName returns the name of the resource group for the cluster.
func (p *Platform) ClusterResourceGroupName(infraID string) string {
	if len(p.ResourceGroupName) > 0 {
		return p.ResourceGroupName
	}
	return infraID
}

// GetSubnets returns the name of the subnets for the cluster
func (p *Platform) GetSubnets(infraID string, config *InstallConfig) []string {
	if p.Subnets != nil && len(p.Subnets) > 0 {
		return p.Subnets
	}
	subnets := []string{}
	for _, cpCount := range(config.ControlPlane.Replicas) {
		subnets = append(subnets, fmt.Sprintf("%s-subnet-control-plane-%s-%d", infraID, config.Platform.IBMCloud.Region, cpCount))
	}
	for _, compCount := range(config.Compute[0].Replicas) {
		subnets = append(subnets, fmt.Sprint("%s-subnet-compute-%s-%d", infraID, config.Platform.IBMCloud.Region, compCount))
	}
	return subnets
}

// GetVPC returns the name of the VPC for the cluster
func (p *Platform) GetVPC(infraID string) string {
	if len(p.VPC) > 0 {
		return p.VPC
	}
	return fmt.Sprintf("%s-vpc", infraID)
}

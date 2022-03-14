package ibmcloud

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/openshift/installer/pkg/asset/installconfig"
)

// https://github.com/kubernetes/kubernetes/blob/368ee4bb8ee7a0c18431cd87ee49f0c890aa53e5/staging/src/k8s.io/legacy-cloud-providers/gce/gce.go#L188
type config struct {
	Global     global     `gcfg:"global"`
	Kubernetes kubernetes `gcfg:"kubernetes"`
	Provider   provider   `gcfg:"provider"`
}

type global struct {
	Version string `gcfg:"version"`
}

type kubernetes struct {
	ConfigFile string `gcfg:"config-file"`
}

type provider struct {
	AccountID                string `gcfg:"accountID"`
	ClusterID                string `gcfg:"clusterID"`
	ClusterDefaultProvider   string `gcfg:"cluster-default-provider"`
	Region                   string `gcfg:"region"`
	G2CredentialsFilePath    string `gcfg:"g2Credentials"`
	G2ResourceGroupName      string `gcfg:"g2ResourceGroupName"`
	G2VPCName                string `gcfg:"g2VpcName"`
	G2WorkerServiceAccountID string `gcfg:"g2workerServiceAccountID"`
	G2VPCSubnetNames         string `gcfg:"g2VpcSubnetNames"`
}

// CloudProviderConfig generates the cloud provider config for the IBMCloud platform.
func CloudProviderConfig(infraID string, accountID string, installConfig installconfig.InstallConfig, controlPlaneZones []string, computeZones []string) (string, error) {
	vpcName := installConfig.Config.Platform.IBMCloud.GetVPCName()
	if vpcName == "" {
		vpcName = fmt.Sprintf("%s-vpc", infraID)
	}
	existingControlPlaneSubnets, err := installConfig.IBMCloud.ControlPlaneSubnets(context.TODO())
	if err != nil {
		return "", err
	}
	existingComputeSubnets, err := installConfig.IBMCloud.ComputeSubnets(context.TODO())
	if err != nil {
		return "", err
	}

	var compiledSubnetNames string
	if len(existingControlPlaneSubnets) > 0 && len(existingComputeSubnets) > 0 {
		var subnetNames []string
		for _, cpSubnet := range existingControlPlaneSubnets {
			subnetNames = append(subnetNames, cpSubnet.Name)
		}
		for _, compSubnet := range existingComputeSubnets {
			subnetNames = append(subnetNames, compSubnet.Name)
		}
		compiledSubnetNames = strings.Join(subnetNames, ",")
	} else if len(existingControlPlaneSubnets) == 0 && len(existingComputeSubnets) == 0 {
		compiledSubnetNames = getVpcSubnetNames(infraID, controlPlaneZones, computeZones)
	} else {
		return "", fmt.Errorf("existing subnets must both be zero or both be > 0. %d ControlPlaneSubnets, %d ComputeSubnets", len(existingControlPlaneSubnets), len(existingComputeSubnets))
	}

	config := &config{
		Global: global{
			Version: "1.1.0",
		},
		Kubernetes: kubernetes{
			ConfigFile: "",
		},
		Provider: provider{
			AccountID:                accountID,
			ClusterID:                infraID,
			ClusterDefaultProvider:   "g2",
			Region:                   installConfig.Config.Platform.IBMCloud.Region,
			G2CredentialsFilePath:    "/etc/vpc/ibmcloud_api_key",
			G2ResourceGroupName:      installConfig.Config.Platform.IBMCloud.ClusterResourceGroupName(infraID),
			G2VPCName:                vpcName,
			G2WorkerServiceAccountID: accountID,
			G2VPCSubnetNames:         compiledSubnetNames,
		},
	}
	buf := &bytes.Buffer{}
	template := template.Must(template.New("ibmcloud cloudproviderconfig").Parse(configTmpl))
	if err := template.Execute(buf, config); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Generate a string of Subnet names for Control Plane and Compute based off the cluster name
func getVpcSubnetNames(infraID string, controlPlaneZones []string, computeZones []string) string {
	var subnetNames []string

	for cpIndex := range controlPlaneZones {
		// Add Control Plane subnet
		subnetNames = append(subnetNames, fmt.Sprintf("%s-subnet-control-plane-%s", infraID, controlPlaneZones[cpIndex]))
	}
	for comIndex := range computeZones {
		// Add Compute subnet
		subnetNames = append(subnetNames, fmt.Sprintf("%s-subnet-compute-%s", infraID, computeZones[comIndex]))
	}
	sort.Strings(subnetNames)
	return strings.Join(subnetNames, ",")
}

var configTmpl = `[global]
version = {{.Global.Version}}
[kubernetes]
config-file = {{ if ne .Kubernetes.ConfigFile "" }}{{ .Kubernetes.ConfigFile }}{{ else }}""{{ end }}
[provider]
accountID = {{.Provider.AccountID}}
clusterID = {{.Provider.ClusterID}}
cluster-default-provider = {{.Provider.ClusterDefaultProvider}}
region = {{.Provider.Region}}
g2Credentials = {{.Provider.G2CredentialsFilePath}}
g2ResourceGroupName = {{.Provider.G2ResourceGroupName}}
g2VpcName = {{.Provider.G2VPCName}}
g2workerServiceAccountID = {{.Provider.G2WorkerServiceAccountID}}
g2VpcSubnetNames = {{.Provider.G2VPCSubnetNames}}

`

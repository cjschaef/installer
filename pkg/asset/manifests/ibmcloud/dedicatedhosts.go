package ibmcloud

import (
	"context"
	"fmt"

	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"

	ibmcloudic "github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
	"github.com/openshift/installer/pkg/types"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
	"github.com/sirupsen/logrus"
)

func generateCAPIDedicatedHosts(installConfig *types.InstallConfig, infraID string, defaultZones []string) ([]capibmcloud.VPCDedicatedHost, error) {
	// Check whether there are any Dedicated Hosts defined in the config.
	if !checkMachinePoolsForDedicatedHosts(installConfig) {
		return nil, nil
	}
	uniqueDedicatedHosts := make(map[string]capibmcloud.VPCDedicatedHost, 0)

	metadata := ibmcloudic.NewMetadata(installConfig)
	client, err := metadata.Client()
	if err != nil {
		return nil, fmt.Errorf("failed creating IBM Cloud client: %w", err)
	}
	region := installConfig.Platform.IBMCloud.Region

	// Start with DefaultMachinePool Dedicated Hosts.
	if installConfig.Platform.IBMCloud != nil && installConfig.Platform.IBMCloud.DefaultMachinePlatform != nil && len(installConfig.Platform.IBMCloud.DefaultMachinePlatform.DedicatedHosts) > 0 {
		// Use default zones, or zones from DefaultMachinePlatform if provided.
		zones := defaultZones
		if len(installConfig.Platform.IBMCloud.DefaultMachinePlatform.Zones) > 0 {
			zones = installConfig.Platform.IBMCloud.DefaultMachinePlatform.Zones
		}

		processedDedicatedHosts, err := processDedicatedHosts(client, installConfig.Platform.IBMCloud.DefaultMachinePlatform.DedicatedHosts, zones, infraID, region, "control-plane")
		if err != nil {
			return nil, fmt.Errorf("failed processing defaultMachinePlatform dedicated hosts: %w", err)
		}
		for name, dHost := range processedDedicatedHosts {
			if _, ok := uniqueDedicatedHosts[name]; !ok {
				logrus.Debugf("added defaultMachinePlatform dedicated host: name=%s, zone=%s", *dHost.Name, dHost.Zone)
				uniqueDedicatedHosts[name] = dHost
			}
		}
	}

	// Check Control Plane Dedicated Hosts.
	if installConfig.ControlPlane.Platform.IBMCloud != nil && len(installConfig.ControlPlane.Platform.IBMCloud.DedicatedHosts) > 0 {
		zones := defaultZones
		if len(installConfig.ControlPlane.Platform.IBMCloud.Zones) > 0 {
			zones = installConfig.ControlPlane.Platform.IBMCloud.Zones
		}

		processedDedicatedHosts, err := processDedicatedHosts(client, installConfig.ControlPlane.Platform.IBMCloud.DedicatedHosts, zones, infraID, region, "control-plane")
		if err != nil {
			return nil, fmt.Errorf("failed processing controlPlane dedicated hosts: %w", err)
		}
		for name, dHost := range processedDedicatedHosts {
			if _, ok := uniqueDedicatedHosts[name]; !ok {
				logrus.Debugf("added controlPlane dedicated host: name=%s, zone=%s", *dHost.Name, dHost.Zone)
				uniqueDedicatedHosts[name] = dHost
			}
		}
	}

	// Check Compute[x] Dedicated Hosts.
	for index, compute := range installConfig.Compute {
		if compute.Platform.IBMCloud != nil && len(compute.Platform.IBMCloud.DedicatedHosts) > 0 {
			zones := defaultZones
			if len(compute.Platform.IBMCloud.Zones) > 0 {
				zones = compute.Platform.IBMCloud.Zones
			}

			processedDedicatedHosts, err := processDedicatedHosts(client, compute.Platform.IBMCloud.DedicatedHosts, zones, infraID, region, "compute")
			if err != nil {
				return nil, fmt.Errorf("failed processing compute[%d] dedicated hosts: %w", index, err)
			}
			for name, dHost := range processedDedicatedHosts {
				if _, ok := uniqueDedicatedHosts[name]; !ok {
					logrus.Debugf("added compute[%d] dedicated host: name=%s, zone=%s", index, *dHost.Name, dHost.Zone)
					uniqueDedicatedHosts[name] = dHost
				}
			}
		}
	}

	// Generate the set of unique Dedicated Hosts (no duplicates).
	dedicatedHosts := make([]capibmcloud.VPCDedicatedHost, 0)
	for _, dHost := range uniqueDedicatedHosts {
		if dHost.Name != nil || dHost.ID != nil {
			dedicatedHosts = append(dedicatedHosts, dHost)
		}
	}
	return dedicatedHosts, nil
}

func checkMachinePoolsForDedicatedHosts(installConfig *types.InstallConfig) bool {
	switch {
	case installConfig.Platform.IBMCloud != nil && installConfig.Platform.IBMCloud.DefaultMachinePlatform != nil && len(installConfig.Platform.IBMCloud.DefaultMachinePlatform.DedicatedHosts) > 0:
		return true
	case len(installConfig.ControlPlane.Platform.IBMCloud.DedicatedHosts) > 0:
		return true
	}

	for _, compute := range installConfig.Compute {
		if len(compute.Platform.IBMCloud.DedicatedHosts) > 0 {
			return true
		}
	}

	return false
}

func processDedicatedHosts(client ibmcloudic.API, dedicatedHosts []ibmcloudtypes.DedicatedHost, zones []string, infraID string, region string, role string) (map[string]capibmcloud.VPCDedicatedHost, error) {
	uniqueDedicatedHosts := make(map[string]capibmcloud.VPCDedicatedHost, 0)
	for index, dHost := range dedicatedHosts {
		// If a profile was provided, assume a new Dedicated Host should be created.
		switch {
		case dHost.Profile != "":
			// NOTE(cjschaef): There is a hard assumption that the Dedicated Host order matches that of the expected Zones. Excess Dedicated Hosts overlap the Zones via modulo comparison.
			zone := zones[index%len(zones)]
			name := dHost.Name
			if name == "" {
				name = fmt.Sprintf("%s-dhost-%s-%s", infraID, role, zone)
			}
			// Prevent adding duplicate Dedicated Hosts.
			if _, ok := uniqueDedicatedHosts[name]; !ok {
				uniqueDedicatedHosts[name] = capibmcloud.VPCDedicatedHost{
					Name:    ptr.To(name),
					Profile: ptr.To(dHost.Profile),
					Zone:    zone,
				}
			}
		case dHost.Name != "":
			// If Profile was not defined, there is a hard assumption that the Dedicated Hosts listed are existing Dedicated Hosts. Attempt lookup of them to find the appropriate Zone and ID.
			dHostDetails, err := client.GetDedicatedHostByName(context.TODO(), dHost.Name, region)
			if err != nil {
				return nil, fmt.Errorf("failure retrieving dedicated host by name %s: %w", dHost.Name, err)
			} else if dHostDetails == nil {
				return nil, fmt.Errorf("failed to find dedicated host by name %s", dHost.Name)
			}
			// Prevent adding duplicate Dedicated Hosts.
			if _, ok := uniqueDedicatedHosts[*dHostDetails.Name]; !ok {
				uniqueDedicatedHosts[*dHostDetails.Name] = capibmcloud.VPCDedicatedHost{
					ID:   dHostDetails.ID,
					Name: dHostDetails.Name,
					Zone: *dHostDetails.Zone.Name,
				}
			}
		default:
			return nil, fmt.Errorf("defaultMachinePlatform dedicated host has no name or profile defined")
		}
	}
	return uniqueDedicatedHosts, nil
}

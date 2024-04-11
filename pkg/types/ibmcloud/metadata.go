package ibmcloud

import (
	"fmt"
	configv1 "github.com/openshift/api/config/v1"
)

// Metadata contains IBM Cloud metadata (e.g. for uninstalling the cluster).
type Metadata struct {
	AccountID         string                             `json:"accountID"`
	BaseDomain        string                             `json:"baseDomain"`
	CISInstanceCRN    string                             `json:"cisInstanceCRN,omitempty"`
	DNSInstanceID     string                             `json:"dnsInstanceID,omitempty"`
	Region            string                             `json:"region,omitempty"`
	ResourceGroupName string                             `json:"resourceGroupName,omitempty"`
	ServiceEndpoints  []configv1.IBMCloudServiceEndpoint `json:"serviceEndpoints,omitempty"`
	Subnets           []string                           `json:"subnets,omitempty"`
	VPC               string                             `json:"vpc,omitempty"`
}

// GetRegionAndEndpointsFlag will return the IBM Cloud region and any service endpoint overrides formatted as the IBM Cloud CAPI command line argument.
func (m *Metadata) GetRegionAndEndpointsFlag() string {
	// If there are no endpoints, return an empty string (rather than just the region).
	if m.ServiceEndpoints == nil || len(m.ServiceEndpoints) == 0 {
		return ""
	}

	flag := m.Region
	for index, endpoint := range m.ServiceEndpoints {
		// IBM Cloud CAPI has pre-defined endpoint service names that do not follow naming scheme, use those instead until they are fixed.
		// TODO(cjschaef): See about opening a CAPI GH issue to link here for this restriction.
		serviceName := endpoint.Name
		if endpoint.Name == configv1.IBMCloudServiceResourceController {
			serviceName = "rc"
		} else if endpoint.Name == configv1.IBMCloudServiceVPC {
			serviceName = "vpc"
		}

		// Format for first (and perhaps only) endpoint is unique, remaining are similar
		if index == 0 {
			flag = fmt.Sprintf("%s:%s=%s", flag, serviceName, endpoint.URL)
		} else {
			flag = fmt.Sprintf("%s,%s=%s", flag, serviceName, endpoint.URL)
		}
	}
	return flag
}

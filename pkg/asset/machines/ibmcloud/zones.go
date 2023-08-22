package ibmcloud

import (
	"context"

        configv1 "github.com/openshift/api/config/v1"

	"github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
)

// AvailabilityZones returns a list of supported zones for the specified region.
func AvailabilityZones(region string, serviceEndpoints []configv1.IBMCloudServiceEndpoint) ([]string, error) {
	ctx := context.TODO()

	var client ibmcloud.API
	var err error
	// If service endpoints were provided get a Client utilizing those endpoints
	if len(serviceEndpoints) > 0 {
		client, err = ibmcloud.NewClientEndpointOverride(serviceEndpoints)
	} else {
		client, err = ibmcloud.NewClient()
	}
	if err != nil {
		return nil, err
	}

	return client.GetVPCZonesForRegion(ctx, region)
}

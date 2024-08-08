package ibmcloud

import (
	"context"

	"github.com/IBM/vpc-go-sdk/vpcv1"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
)

// FindSSHKey attempts to find an IBM Cloud VPC SSH Key with the matching public key.
func FindSSHKey(publicSSHKey string, region string, serviceEndpoints []configv1.IBMCloudServiceEndpoint) (*vpcv1.Key, error) {
	ctx := context.TODO()

	client, err := ibmcloud.NewClient(serviceEndpoints)
	if err != nil {
		return nil, err
	}

	return client.GetSSHKeyByPublicKey(ctx, publicSSHKey, region)
}

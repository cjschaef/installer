package clusterapi

import (
	"context"

	"github.com/openshift/installer/pkg/infrastructure/clusterapi"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
)

var _ clusterapi.PreProvider = Provider{}
var _ clusterapi.Provider = Provider{}

// Provider is the IBM Cloud implementation of the clusterapi InfraProvider.
type Provider struct {}

// Name returns the IBM Cloud provider name.
func (p Provider) Name() string {
	return ibmcloudtypes.Name
}

// PreProvision creates the IBM Cloud objects required prior to running capibmcloud.
func (p Provider) PreProvision(ctx context.Context, in clusterapi.PreProvisionInput) error {
	return nil
}

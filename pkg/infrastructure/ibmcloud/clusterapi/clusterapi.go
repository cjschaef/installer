package clusterapi

import (
	"context"

	"github.com/openshift/installer/pkg/infrastructure/clusterapi"
	ibmcloudtypes "github.com/openshift/installer/pkg/types/ibmcloud"
)

// Provider is the IBM Cloud implementation of the clusterapi InfraProvider.
type Provider struct {
	clusterapi.InfraProvider
}

var _ clusterapi.PreProvider = Provider{}

// Name returns the IBM Cloud provider name.
func (p Provider) Name() string {
	return ibmcloudtypes.Name
}

// PreProvision creates the IBM Cloud objects required prior to running capibmcloud.
func (p Provider) PreProvision(ctx context.Context, in clusterapi.PreProvisionInput) error {
	return nil
}
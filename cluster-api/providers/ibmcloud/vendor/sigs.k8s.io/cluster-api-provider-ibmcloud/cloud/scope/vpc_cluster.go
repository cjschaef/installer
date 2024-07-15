/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scope

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/globaltaggingv1"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"

	"k8s.io/klog/v2/textlogger"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"

	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"

	infrav1beta2 "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/authenticator"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/cos"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/globaltagging"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/resourcecontroller"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/resourcemanager"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/vpc"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/endpoints"
)

const (
	// LOGDEBUGLEVEL indicates the debug level of the logs.
	LOGDEBUGLEVEL = 5
)

// VPCClusterScopeParams defines the input parameters used to create a new VPCClusterScope.
type VPCClusterScopeParams struct {
	Client          client.Client
	Cluster         *capiv1beta1.Cluster
	IBMVPCCluster   *infrav1beta2.IBMVPCCluster
	Logger          logr.Logger
	ServiceEndpoint []endpoints.ServiceEndpoint

	IBMVPCClient vpc.Vpc
}

// VPCClusterScope defines a scope defined around a VPC Cluster.
type VPCClusterScope struct {
	logr.Logger
	Client      client.Client
	patchHelper *patch.Helper

	COSClient                cos.Cos
	GlobalTaggingClient      globaltagging.GlobalTagging
	ResourceControllerClient resourcecontroller.ResourceController
	ResourceManagerClient    resourcemanager.ResourceManager
	VPCClient                vpc.Vpc

	Cluster         *capiv1beta1.Cluster
	IBMVPCCluster   *infrav1beta2.IBMVPCCluster
	ServiceEndpoint []endpoints.ServiceEndpoint
}

// NewVPCClusterScope creates a new VPCClusterScope from the supplied parameters.
func NewVPCClusterScope(params VPCClusterScopeParams) (*VPCClusterScope, error) {
	if params.Client == nil {
		err := errors.New("error failed to generate new scope from nil Client")
		return nil, err
	}
	if params.Cluster == nil {
		err := errors.New("error failed to generate new scope from nil Cluster")
		return nil, err
	}
	if params.IBMVPCCluster == nil {
		err := errors.New("error failed to generate new scope from nil IBMVPCCluster")
		return nil, err
	}
	if params.Logger == (logr.Logger{}) {
		params.Logger = textlogger.NewLogger(textlogger.NewConfig())
	}

	helper, err := patch.NewHelper(params.IBMVPCCluster, params.Client)
	if err != nil {
		return nil, fmt.Errorf("error failed to init patch helper: %w", err)
	}

	vpcEndpoint := endpoints.FetchVPCEndpoint(params.IBMVPCCluster.Spec.Region, params.ServiceEndpoint)
	vpcClient, err := vpc.NewService(vpcEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error failed to create IBM VPC client: %w", err)
	}

	if params.IBMVPCCluster.Spec.Network == nil || params.IBMVPCCluster.Spec.Region == "" {
		return nil, fmt.Errorf("error failed to generate vpc client as Network or Region is nil")
	}

	if params.Logger.V(LOGDEBUGLEVEL).Enabled() {
		core.SetLoggingLevel(core.LevelDebug)
	}

	auth, err := authenticator.GetAuthenticator()
	if err != nil {
		return nil, fmt.Errorf("error failed to create authenticator: %w", err)
	}

	// Create Global Tagging client.
	gtOptions := globaltagging.ServiceOptions{
		GlobalTaggingV1Options: &globaltaggingv1.GlobalTaggingV1Options{
			Authenticator: auth,
		},
	}
	gtEndpoint := endpoints.FetchEndpoints(string(endpoints.GlobalTagging), params.ServiceEndpoint)
	if gtEndpoint != "" {
		gtOptions.URL = gtEndpoint
		params.Logger.V(3).Info("Overriding the default global tagging endpoint", "GlobalTaggingEndpoing", gtEndpoint)
	}
	globalTaggingClient, err := globaltagging.NewService(gtOptions)
	if err != nil {
		return nil, fmt.Errorf("error failed to create global tagging client: %w", err)
	}

	// Create Resource Controller client.
	rcOptions := resourcecontroller.ServiceOptions{
		ResourceControllerV2Options: &resourcecontrollerv2.ResourceControllerV2Options{
			Authenticator: auth,
		},
	}
	// Fetch the resource controller endpoint.
	rcEndpoint := endpoints.FetchEndpoints(string(endpoints.RC), params.ServiceEndpoint)
	if rcEndpoint != "" {
		rcOptions.URL = rcEndpoint
		params.Logger.V(3).Info("Overriding the default resource controller endpoint", "ResourceControllerEndpoint", rcEndpoint)
	}
	resourceControllerClient, err := resourcecontroller.NewService(rcOptions)
	if err != nil {
		return nil, fmt.Errorf("error failed to create resource controller client: %w", err)
	}

	// Create Resource Manager client.
	rmOptions := &resourcemanagerv2.ResourceManagerV2Options{
		Authenticator: auth,
	}
	// Fetch the ResourceManager endpoint.
	rmEndpoint := endpoints.FetchEndpoints(string(endpoints.RM), params.ServiceEndpoint)
	if rmEndpoint != "" {
		rmOptions.URL = rmEndpoint
		params.Logger.V(3).Info("Overriding  the default resource manager endpoint", "ResourceManagerEndpoint", rmEndpoint)
	}
	resourceManagerClient, err := resourcemanager.NewService(rmOptions)
	if err != nil {
		return nil, fmt.Errorf("error failed to create resource manager client: %w", err)
	}

	clusterScope := &VPCClusterScope{
		Logger:                   params.Logger,
		Client:                   params.Client,
		patchHelper:              helper,
		Cluster:                  params.Cluster,
		IBMVPCCluster:            params.IBMVPCCluster,
		ServiceEndpoint:          params.ServiceEndpoint,
		GlobalTaggingClient:      globalTaggingClient,
		ResourceControllerClient: resourceControllerClient,
		ResourceManagerClient:    resourceManagerClient,
		VPCClient:                vpcClient,
	}
	return clusterScope, nil
}

// PatchObject persists the cluster configuration and status.
func (s *VPCClusterScope) PatchObject() error {
	return s.patchHelper.Patch(context.TODO(), s.IBMVPCCluster)
}

// Close closes the current scope persisting the cluster configuration and status.
func (s *VPCClusterScope) Close() error {
	return s.PatchObject()
}

// Name returns the CAPI cluster name.
func (s *VPCClusterScope) Name() string {
	return s.Cluster.Name
}

// ResourceGroup returns the cluster's ResourceGroup.
func (s *VPCClusterScope) ResourceGroup() string {
	return s.IBMVPCCluster.Spec.ResourceGroup
}

// NetworkResourceGroup returns the Network's ResourceGroup, which can be unique than the cluster's ResourceGroup (BYON).
func (s *VPCClusterScope) NetworkResourceGroup() string {
	if s.IBMVPCCluster.Spec.Network != nil && s.IBMVPCCluster.Spec.Network.ResourceGroup != nil {
		return *s.IBMVPCCluster.Spec.Network.ResourceGroup
	}
	return s.IBMVPCCluster.Spec.ResourceGroup
}

// InfraCluster returns the IBMPowerVS infrastructure cluster object name.
func (s *VPCClusterScope) InfraCluster() string {
	return s.IBMVPCCluster.Name
}

// APIServerPort returns the APIServerPort to use when creating the ControlPlaneEndpoint.
func (s *VPCClusterScope) APIServerPort() int32 {
	if s.Cluster.Spec.ClusterNetwork != nil && s.Cluster.Spec.ClusterNetwork.APIServerPort != nil {
		return *s.Cluster.Spec.ClusterNetwork.APIServerPort
	}
	return infrav1beta2.DefaultAPIServerPort
}

// SetStatus set the IBMVPCCluster status for provided ResourceType.
func (s *VPCClusterScope) SetStatus(resourceType infrav1beta2.ResourceType, resource infrav1beta2.GenericResourceReference) {
	s.V(3).Info("Setting status", "resourceType", resourceType, "resource", resource)
	switch resourceType {
	case infrav1beta2.ResourceTypeResourceGroup:
		if s.IBMVPCCluster.Status.ResourceGroup == nil {
			s.IBMVPCCluster.Status.ResourceGroup = &resource
			return
		}
		s.IBMVPCCluster.Status.ResourceGroup.Set(resource)
	default:
		s.Info("unsupported resource type")
	}
}

// SetLoadBalancerStatus sets the Load Balancer status.
func (s *VPCClusterScope) SetLoadBalancerStatus(loadBalancer infrav1beta2.VPCLoadBalancerStatus) {
	s.V(3).Info("Setting status", "resourceType", infrav1beta2.ResourceTypeLoadBalancer, "resource", loadBalancer)
	if s.IBMVPCCluster.Status.NetworkStatus == nil {
		s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{}
	}
	if s.IBMVPCCluster.Status.NetworkStatus.LoadBalancers == nil {
		s.IBMVPCCluster.Status.NetworkStatus.LoadBalancers = make(map[string]*infrav1beta2.VPCLoadBalancerStatus)
	}
	if lb, ok := s.IBMVPCCluster.Status.NetworkStatus.LoadBalancers[*loadBalancer.ID]; ok {
		lb.ID = loadBalancer.ID
		lb.State = loadBalancer.State
		lb.Hostname = loadBalancer.Hostname
	} else {
		s.IBMVPCCluster.Status.NetworkStatus.LoadBalancers[*loadBalancer.ID] = ptr.To(loadBalancer)
	}
}

// SetVPCResourceStatus sets the IBMVPCCluster status for VPC resources.
func (s *VPCClusterScope) SetVPCResourceStatus(resourceType infrav1beta2.ResourceType, resource infrav1beta2.VPCResourceStatus) {
	s.V(3).Info("Setting status", "resourceType", resourceType, "resource", resource)
	switch resourceType {
	case infrav1beta2.ResourceTypeVPC:
		if s.IBMVPCCluster.Status.NetworkStatus == nil {
			s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{
				VPC: &resource,
			}
			return
		} else if s.IBMVPCCluster.Status.NetworkStatus.VPC == nil {
			s.IBMVPCCluster.Status.NetworkStatus.VPC = ptr.To(resource)
			return
		}
		s.IBMVPCCluster.Status.NetworkStatus.VPC.Set(resource)
	case infrav1beta2.ResourceTypeCustomImage:
		if s.IBMVPCCluster.Status.ImageStatus == nil {
			s.IBMVPCCluster.Status.ImageStatus = ptr.To(resource)
			return
		}
		s.IBMVPCCluster.Status.ImageStatus.Set(resource)
	case infrav1beta2.ResourceTypeControlPlaneSubnet:
		if s.IBMVPCCluster.Status.NetworkStatus == nil {
			s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets[resource.Name]; ok {
			subnet.Set(resource)
		} else {
			s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets[resource.Name] = ptr.To(resource)
		}
	case infrav1beta2.ResourceTypeComputeSubnet:
		if s.IBMVPCCluster.Status.NetworkStatus == nil {
			s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets[resource.Name]; ok {
			subnet.Set(resource)
		} else {
			s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets[resource.Name] = ptr.To(resource)
		}
	case infrav1beta2.ResourceTypeSecurityGroup:
		if s.IBMVPCCluster.Status.NetworkStatus == nil {
			s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups == nil {
			s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		if securityGroup, ok := s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups[resource.Name]; ok {
			securityGroup.Set(resource)
		} else {
			s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups[resource.Name] = ptr.To(resource)
		}
	default:
		s.Info("unsupported vpc resource type")
	}
}

/*
// NetworkSpec returns the cluster NetworkSpec.
func (s *VPCClusterScope) NetworkSpec() *infrav1beta2.VPCNetworkSpec {
	return s.IBMVPCCluster.Spec.NetworkSpec
}.
*/

// VPC returns the cluster VPC information.
func (s *VPCClusterScope) VPC() *infrav1beta2.VPCResource {
	if s.IBMVPCCluster.Spec.Network == nil {
		return nil
	}
	return s.IBMVPCCluster.Spec.Network.VPC
}

// GetVPCID returns the VPC id.
func (s *VPCClusterScope) GetVPCID() (*string, error) {
	if s.IBMVPCCluster.Status.NetworkStatus != nil && s.IBMVPCCluster.Status.NetworkStatus.VPC != nil {
		return ptr.To(s.IBMVPCCluster.Status.NetworkStatus.VPC.ID), nil
	}
	if s.IBMVPCCluster.Spec.Network != nil && s.IBMVPCCluster.Spec.Network.VPC != nil {
		if s.IBMVPCCluster.Spec.Network.VPC.ID != nil {
			return s.IBMVPCCluster.Spec.Network.VPC.ID, nil
		} else if s.IBMVPCCluster.Spec.Network.VPC.Name != nil {
			vpc, err := s.VPCClient.GetVPCByName(*s.IBMVPCCluster.Spec.Network.VPC.Name)
			if err != nil {
				return nil, err
			}
			// Check if VPC was found and has an ID
			if vpc != nil && vpc.ID != nil {
				// Set VPC ID to shortcut future lookups
				s.IBMVPCCluster.Spec.Network.VPC.ID = vpc.ID
				return s.IBMVPCCluster.Spec.Network.VPC.ID, nil
			}
		}
	}
	return nil, nil
}

// GetSubnetID returns the ID of a subnet, provided the name.
func (s *VPCClusterScope) GetSubnetID(name string) (*string, error) {
	// Check Status first
	if s.IBMVPCCluster.Status.NetworkStatus != nil {
		if s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets != nil {
			if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets[name]; ok {
				return &subnet.ID, nil
			}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets != nil {
			if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets[name]; ok {
				return &subnet.ID, nil
			}
		}
	}
	// Otherwise, if no Status, or not found, attempt to look it up
	subnet, err := s.VPCClient.GetVPCSubnetByName(name)
	if err != nil {
		return nil, err
	}
	if subnet == nil {
		return nil, nil
	}
	return subnet.ID, nil
}

// GetSubnetIDs returns all of the subnet Id's, duplicates removed.
func (s *VPCClusterScope) GetSubnetIDs() ([]string, error) { //nolint: gocyclo
	subnetMap := make(map[string]bool, 0)

	checkSubnets := func(subnets []infrav1beta2.Subnet) error {
		for _, subnet := range subnets {
			if subnet.ID != nil {
				if _, exists := subnetMap[*subnet.ID]; !exists {
					subnetMap[*subnet.ID] = true
				}
			} else if subnet.Name != nil {
				subnetID, err := s.GetSubnetID(*subnet.Name)
				if err != nil {
					return err
				} else if subnetID == nil {
					// Likely the subnet does not exist yet (we have name, but no ID can be found from IBM Cloud API), skip to next subnet
					continue
				}
				if _, exists := subnetMap[*subnetID]; !exists {
					subnetMap[*subnetID] = true
				}
			}
		}
		return nil
	}

	// Try to get subnet Id's from Status first, then try Spec for Id's
	if s.IBMVPCCluster.Status.NetworkStatus != nil {
		if s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets != nil {
			for _, subnet := range s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets {
				if _, exists := subnetMap[subnet.ID]; !exists {
					subnetMap[subnet.ID] = true
				}
			}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets != nil {
			for _, subnet := range s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets {
				if _, exists := subnetMap[subnet.ID]; !exists {
					subnetMap[subnet.ID] = true
				}
			}
		}
	} else if s.IBMVPCCluster.Spec.Network != nil {
		// Check for Id's in ControlPlaneSubnetSpec, or try Id lookups by name
		if s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets != nil {
			err := checkSubnets(s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets)
			if err != nil {
				return nil, err
			}
		}
		if s.IBMVPCCluster.Spec.Network.WorkerSubnets != nil {
			err := checkSubnets(s.IBMVPCCluster.Spec.Network.WorkerSubnets)
			if err != nil {
				return nil, err
			}
		}
	}

	// Transfer subnets from map (used to prevent duplicate entries) to slice
	subnets := make([]string, 0)
	for id := range subnetMap {
		subnets = append(subnets, id)
	}
	return subnets, nil
}

// getSecurityGroupID returns the Security Group ID from the SecurityGroup resource or attempts to look it up in Status. It does not attempt to find the ID using vpcv1 API calls.
func (s *VPCClusterScope) getSecurityGroupID(securityGroup infrav1beta2.VPCSecurityGroup) *string {
	if securityGroup.ID != nil {
		return securityGroup.ID
	}
	// If the Security Group name is not set (nor the ID), that is a problem, but return nil as we only check known information
	if securityGroup.Name == nil {
		return nil
	}
	return s.getSecurityGroupIDFromStatus(*securityGroup.Name)
}

// getSecurityGroupIDFromStatus returns the Security Group ID from the NetworkStatus for the specified Security Group name, if it possible (it has been cached in Status).
func (s *VPCClusterScope) getSecurityGroupIDFromStatus(name string) *string {
	if s.IBMVPCCluster.Status.NetworkStatus != nil && s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups != nil {
		if sg, ok := s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups[name]; ok {
			return ptr.To(sg.ID)
		}
	}
	return nil
}

/*
// PublicLoadBalancer returns the cluster public loadBalancer information.
func (s *VPCClusterScope) PublicLoadBalancer() *infrav1beta2.VPCLoadBalancerSpec {
	// if the user did not specify any loadbalancer then return the public loadbalancer created by the controller.
	if len(s.IBMVPCCluster.Spec.LoadBalancers) == 0 {
		return &infrav1beta2.VPCLoadBalancerSpec{
			Name:   *s.GetServiceName(infrav1beta2.ResourceTypeLoadBalancer),
			Public: ptr.To(true),
		}
	}
	for _, lb := range s.IBMVPCCluster.Spec.LoadBalancers {
		if lb.Public != nil && *lb.Public {
			return &lb
		}
	}
	return nil
}

// SetLoadBalancerStatus set the loadBalancer id.
func (s *VPCClusterScope) SetLoadBalancerStatus(name string, loadBalancer infrav1beta2.VPCLoadBalancerStatus) {
	s.V(3).Info("Setting status", "name", name, "status", loadBalancer)
	if s.IBMVPCCluster.Status.LoadBalancers == nil {
		s.IBMVPCCluster.Status.LoadBalancers = make(map[string]infrav1beta2.VPCLoadBalancerStatus)
	}
	if val, ok := s.IBMVPCCluster.Status.LoadBalancers[name]; ok {
		if val.ControllerCreated != nil && *val.ControllerCreated {
			loadBalancer.ControllerCreated = val.ControllerCreated
		}
	}
	s.IBMVPCCluster.Status.LoadBalancers[name] = loadBalancer
}

// GetLoadBalancerID returns the loadBalancer.
func (s *VPCClusterScope) GetLoadBalancerID(loadBalancerName string) *string {
	if s.IBMVPCCluster.Status.LoadBalancers == nil {
		return nil
	}
	if val, ok := s.IBMVPCCluster.Status.LoadBalancers[loadBalancerName]; ok {
		return val.ID
	}
	return nil
}

// GetLoadBalancerState will return the state for the load balancer.
func (s *VPCClusterScope) GetLoadBalancerState(name string) *infrav1beta2.VPCLoadBalancerState {
	if s.IBMVPCCluster.Status.LoadBalancers == nil {
		return nil
	}
	if val, ok := s.IBMVPCCluster.Status.LoadBalancers[name]; ok {
		return &val.State
	}
	return nil
}

// GetLoadBalancerHostName will return the hostname of load balancer.
func (s *VPCClusterScope) GetLoadBalancerHostName(name string) *string {
	if s.IBMVPCCluster.Status.LoadBalancers == nil {
		return nil
	}
	if val, ok := s.IBMVPCCluster.Status.LoadBalancers[name]; ok {
		return val.Hostname
	}
	return nil
}.
*/

// GetNetworkResourceGroupID returns the Resource Group ID, if it is present for the Network Resources. Otherwise, it defaults to the cluster's Resource Group ID.
func (s *VPCClusterScope) GetNetworkResourceGroupID() (string, error) {
	// Check if the ID is available from Status first
	if s.IBMVPCCluster.Status.NetworkStatus != nil && s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup != nil && s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup.ID != "" {
		return s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup.ID, nil
	}
	// Collect the Network's Resource Group ID if it is defined in Spec.NetworkSpec
	if s.IBMVPCCluster.Spec.Network != nil && s.IBMVPCCluster.Spec.Network.ResourceGroup != nil {
		// Retrieve the Resource Group based on the name
		resourceGroup, err := s.ResourceManagerClient.GetResourceGroupByName(*s.IBMVPCCluster.Spec.Network.ResourceGroup)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve network Resource Group Id by name: %w", err)
		}

		// Populate the Network Status' Resource Group to shortcut future lookups.
		if s.IBMVPCCluster.Status.NetworkStatus == nil {
			s.IBMVPCCluster.Status.NetworkStatus = &infrav1beta2.VPCNetworkStatus{}
		}
		if s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup == nil {
			s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup = &infrav1beta2.GenericResourceReference{}
		}
		s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup.Set(infrav1beta2.GenericResourceReference{
			ID: *resourceGroup.ID,
		})
		return s.IBMVPCCluster.Status.NetworkStatus.ResourceGroup.ID, nil
	}
	// Otherwise, default to using the cluster's Resource Group ID
	return s.GetResourceGroupID()
}

// GetResourceGroupID returns the resource group id if it present under spec or status field of IBMVPCCluster object
// or returns empty string.
func (s *VPCClusterScope) GetResourceGroupID() (string, error) {
	// Check if the ID is available from Status first
	if s.IBMVPCCluster.Status.ResourceGroup != nil && s.IBMVPCCluster.Status.ResourceGroup.ID != "" {
		return s.IBMVPCCluster.Status.ResourceGroup.ID, nil
	}
	// If the Resource Group is not defined in Spec, we generate the name based on the cluster name
	resourceGroupName := s.IBMVPCCluster.Spec.ResourceGroup
	if resourceGroupName == "" {
		resourceGroupName = s.IBMVPCCluster.Name
	}
	// Retrieve the Resource Group based on the name
	resourceGroup, err := s.ResourceManagerClient.GetResourceGroupByName(resourceGroupName)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve resource group by name; %w", err)
	}
	if resourceGroup == nil || resourceGroup.ID == nil {
		return "", fmt.Errorf("error failed to find resource group or id")
	}
	return *resourceGroup.ID, nil
}

// ReconcileResourceGroup reconciles resource group to fetch resource group id.
func (s *VPCClusterScope) ReconcileResourceGroup() error {
	// Verify if resource group id is set in spec or status field of IBMVPCluster object.
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		return err
	}

	s.Info("Fetched resource group id from cloud", "resourceGroupID", resourceGroupID)
	// Set the status of IBMVPCCluster object with resource group id.
	s.SetStatus(infrav1beta2.ResourceTypeResourceGroup, infrav1beta2.GenericResourceReference{
		ID: resourceGroupID,
	})
	return nil
}

// ReconcileVPC reconciles VPC.
func (s *VPCClusterScope) ReconcileVPC() (bool, error) {
	// if VPC id is set means the VPC is already created
	vpcID, err := s.GetVPCID()
	if err != nil {
		return false, err
	}
	if vpcID != nil {
		s.Info("VPC id is set", "id", vpcID)
		vpcDetails, _, err := s.VPCClient.GetVPC(&vpcv1.GetVPCOptions{
			ID: vpcID,
		})
		if err != nil {
			return false, err
		}
		if vpcDetails == nil {
			return false, fmt.Errorf("failed to get VPC with id %s", *vpcID)
		}
		s.Info("Found VPC with provided id")

		requeue := true
		if vpcDetails.Status != nil && *vpcDetails.Status == string(vpcv1.VPCStatusAvailableConst) {
			requeue = false
		}
		s.SetVPCResourceStatus(infrav1beta2.ResourceTypeVPC, infrav1beta2.VPCResourceStatus{
			ID:   *vpcID,
			Name: *vpcDetails.Name,
			// Ready status will be invert of the need to requeue
			Ready: !requeue,
		})
		return requeue, nil
	}

	// create VPC
	s.Info("Creating a VPC")
	vpcDetails, err := s.createVPC()
	if err != nil {
		return false, err
	}
	s.Info("Successfully created VPC")
	s.SetVPCResourceStatus(infrav1beta2.ResourceTypeVPC, infrav1beta2.VPCResourceStatus{
		ID:    *vpcDetails,
		Name:  *s.GetServiceName(infrav1beta2.ResourceTypeVPC),
		Ready: false,
	})
	return true, nil
}

// createVPC creates VPC.
func (s *VPCClusterScope) createVPC() (*string, error) {
	/// We will use the cluster Resource Group ID, as we expect to create all resources in that Resource Group.
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		return nil, fmt.Errorf("error getting resource group id: %w", err)
	}
	if resourceGroupID == "" {
		s.Info("failed to create vpc, failed to fetch resource group id")
		return nil, fmt.Errorf("error getting resource group id for resource group %v, id is empty", s.ResourceGroup())
	}
	addressPrefixManagement := "auto"
	vpcOption := &vpcv1.CreateVPCOptions{
		ResourceGroup:           &vpcv1.ResourceGroupIdentity{ID: &resourceGroupID},
		Name:                    s.GetServiceName(infrav1beta2.ResourceTypeVPC),
		AddressPrefixManagement: &addressPrefixManagement,
	}
	vpcDetails, _, err := s.VPCClient.CreateVPC(vpcOption)
	if err != nil {
		return nil, err
	}
	if err = s.TagResource(s.IBMVPCCluster.Name, *vpcDetails.CRN); err != nil {
		return nil, fmt.Errorf("error tagging VPC: %w", err)
	}

	return vpcDetails.ID, nil
}

// ReconcileVPCCustomImage reconciles the VPC Custom Image.
func (s *VPCClusterScope) ReconcileVPCCustomImage() (bool, error) {
	var imageID *string
	// Attempt to collect VPC Custom Image info from Status
	if s.IBMVPCCluster.Status.ImageStatus != nil {
		if s.IBMVPCCluster.Status.ImageStatus.ID != "" {
			imageID = ptr.To(s.IBMVPCCluster.Status.ImageStatus.ID)
		} else if s.IBMVPCCluster.Status.ImageStatus.Name != "" {
			image, err := s.VPCClient.GetImageByName(s.IBMVPCCluster.Status.ImageStatus.Name)
			if err != nil {
				return false, fmt.Errorf("error checking vpc custom image by name: %w", err)
			}
			// If the image was found via name, we should be able to get its ID.
			if image != nil {
				imageID = image.ID
			}
		}
	}

	// Check status of VPC Custom Image
	if imageID != nil {
		image, _, err := s.VPCClient.GetImage(&vpcv1.GetImageOptions{
			ID: imageID,
		})
		if err != nil {
			return false, fmt.Errorf("error retrieving vpc custom image by id: %w", err)
		}
		if image == nil {
			return false, fmt.Errorf("error failed to retrieve vpc custom image with id %s", *imageID)
		}
		s.Info("Found VPC Custom Image with provided id")

		requeue := true
		if image.Status != nil && *image.Status == string(vpcv1.ImageStatusAvailableConst) {
			requeue = false
		}
		s.SetVPCResourceStatus(infrav1beta2.ResourceTypeCustomImage, infrav1beta2.VPCResourceStatus{
			ID:   *imageID,
			Name: *image.Name,
			// Ready status will be invert of the need to requeue
			Ready: !requeue,
		})
		return requeue, nil
	}

	// Check if the ImageSpec was defined, as it contains all the data necessary to reoncile
	if s.IBMVPCCluster.Spec.Image == nil {
		return false, fmt.Errorf("error failed to reconcile vpc custom image, no image spec defined")
	}

	// Create Custom Image
	s.Info("Creating a VPC Custom Image")
	image, err := s.createCustomImage()
	if err != nil {
		return false, fmt.Errorf("error failure trying to create vpc custom image: %w", err)
	} else if image == nil {
		return false, fmt.Errorf("error no vpc custom image creation results")
	}

	s.Info("Successfully created VPC Custom Image")
	s.SetVPCResourceStatus(infrav1beta2.ResourceTypeCustomImage, infrav1beta2.VPCResourceStatus{
		ID:    *image.ID,
		Name:  *image.Name,
		Ready: false,
	})
	return true, nil
}

// createCustomImage will create a new VPC Custom Image.
func (s *VPCClusterScope) createCustomImage() (*vpcv1.Image, error) {
	if s.IBMVPCCluster.Spec.Image == nil {
		return nil, fmt.Errorf("error failed to create vpc custom image, no image spec defined")
	}

	// Collect Resource Group ID
	var resourceGroupID *string
	// Check Resource Group in ImageSpec
	if s.IBMVPCCluster.Spec.Image.ResourceGroup != nil {
		if s.IBMVPCCluster.Spec.Image.ResourceGroup.ID != "" {
			resourceGroupID = ptr.To(s.IBMVPCCluster.Spec.Image.ResourceGroup.ID)
		} else if s.IBMVPCCluster.Spec.Image.ResourceGroup.Name != nil {
			id, err := s.ResourceManagerClient.GetResourceGroupByName(*s.IBMVPCCluster.Spec.Image.ResourceGroup.Name)
			if err != nil {
				return nil, fmt.Errorf("error retrieving resource group by name: %w", err)
			}
			resourceGroupID = id.ID
		}
	} else {
		// We will use the cluster Resource Group ID, as we expect to create all resources in that Resource Group.
		id, err := s.GetResourceGroupID()
		if err != nil {
			return nil, fmt.Errorf("error retrieving resource group id: %w", err)
		}
		resourceGroupID = ptr.To(id)
	}

	// We must have an OperatingSystem value supplied in order to create the Custom Image.
	// NOTE(cjschaef): Perhaps we could try defaulting this value, so it isn't required for Custom Image creation.
	if s.IBMVPCCluster.Spec.Image.OperatingSystem == nil {
		return nil, fmt.Errorf("error failed to create vpc custom image due to missing operatingSystem")
	}

	// Build the COS Object URL using the ImageSpec
	fileHRef, err := s.buildCOSObjectHRef()
	if err != nil {
		return nil, fmt.Errorf("error building vpc custom image file href: %w", err)
	} else if fileHRef == nil {
		return nil, fmt.Errorf("error failed to build vpc custom image file href")
	}

	options := &vpcv1.CreateImageOptions{
		ImagePrototype: &vpcv1.ImagePrototype{
			Name: s.IBMVPCCluster.Spec.Image.Name,
			File: &vpcv1.ImageFilePrototype{
				Href: fileHRef,
			},
			OperatingSystem: &vpcv1.OperatingSystemIdentity{
				Name: s.IBMVPCCluster.Spec.Image.OperatingSystem,
			},
			ResourceGroup: &vpcv1.ResourceGroupIdentity{
				ID: resourceGroupID,
			},
		},
	}

	imageDetails, _, err := s.VPCClient.CreateImage(options)
	if err != nil {
		return nil, fmt.Errorf("error unknown failure creating vpc custom image: %w", err)
	}
	if imageDetails == nil || imageDetails.ID == nil || imageDetails.Name == nil || imageDetails.CRN == nil {
		return nil, fmt.Errorf("error failed creating custom image")
	}

	if err := s.TagResource(s.IBMVPCCluster.Name, *imageDetails.CRN); err != nil {
		return nil, fmt.Errorf("error failure tagging vpc custom image: %w", err)
	}
	return imageDetails, nil
}

// buildCOSObjectHRef will build the HRef path to a COS Object that can be used for VPC Custom Image creation.
func (s *VPCClusterScope) buildCOSObjectHRef() (*string, error) {
	// We need COS details in order to create the Custom Image from.
	if s.IBMVPCCluster.Spec.Image.COSInstance == nil || s.IBMVPCCluster.Spec.Image.COSBucket == nil || s.IBMVPCCluster.Spec.Image.COSObject == nil {
		return nil, fmt.Errorf("error failed to build cos object href, cos details missing")
	}

	// Get COS Bucket Region, defaulting to cluster Region if not specified.
	bucketRegion := s.IBMVPCCluster.Spec.Region
	if s.IBMVPCCluster.Spec.Image.COSBucketRegion != nil {
		bucketRegion = *s.IBMVPCCluster.Spec.Image.COSBucketRegion
	}

	href := fmt.Sprintf("cos://%s/%s/%s", bucketRegion, *s.IBMVPCCluster.Spec.Image.COSBucket, *s.IBMVPCCluster.Spec.Image.COSObject)
	s.Info("building image ref", "href", href)
	// Expected HRef structure:
	//   cos://<bucket_region>/<bucket_name>/<object_name>
	return ptr.To(href), nil
}

// findOrCreatePublicGateway will attempt to find if there is an existing Public Gateway for a specific zone, for the cluster (in cluster's/Network's Resource Group and VPC), or create a new one. Only one Public Gateway is required in each zone, for any subnets in that zone.
func (s *VPCClusterScope) findOrCreatePublicGateway(zone string) (*vpcv1.PublicGateway, error) {
	publicGatewayName := fmt.Sprintf("%s-%s", *s.GetServiceName(infrav1beta2.ResourceTypePublicGateway), zone)
	// We will use the cluster Resource Group ID, as we expect to create all resources (Public Gateways and Subnets) in that Resource Group.
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		return nil, err
	}
	publicGateway, err := s.VPCClient.GetPublicGatewayByName(publicGatewayName, resourceGroupID)
	if err != nil {
		return nil, err
	}
	// If we found the Public Gateway, with an ID, for the zone, return it.
	// NOTE(cjschaef): We may wish to confirm the PublicGateway, by checking Tags (Global Tagging), but this might be sufficient, as we don't expect to .
	if publicGateway != nil && publicGateway.ID != nil {
		return publicGateway, nil
	}

	// Otherwise, create a new Public Gateway for the zone.
	vpcID, err := s.GetVPCID()
	if err != nil {
		return nil, err
	}
	if vpcID == nil {
		return nil, fmt.Errorf("error failed to get vpc id for public gateway creation")
	}

	publicGatewayDetails, _, err := s.VPCClient.CreatePublicGateway(&vpcv1.CreatePublicGatewayOptions{
		Name: ptr.To(publicGatewayName),
		ResourceGroup: &vpcv1.ResourceGroupIdentity{
			ID: ptr.To(resourceGroupID),
		},
		VPC: &vpcv1.VPCIdentity{
			ID: vpcID,
		},
		Zone: &vpcv1.ZoneIdentity{
			Name: ptr.To(zone),
		},
	})
	if err != nil {
		return nil, err
	}
	if publicGatewayDetails == nil {
		return nil, fmt.Errorf("error failed creating public gateway for zone %s", zone)
	} else if publicGatewayDetails.ID == nil {
		s.Info("error failed creating public gateway, no ID", "name", publicGatewayName)
		return nil, fmt.Errorf("error failed creating public gateway, no ID")
	} else if publicGatewayDetails.CRN == nil {
		s.Info("error failed creating public gateway, no CRN", "name", publicGatewayName)
		return nil, fmt.Errorf("error failed creating public gateway, no CRN")
	}
	s.Info("created public gateway", "id", publicGatewayDetails.ID)

	// Add a tag to the public gateway for the cluster
	err = s.TagResource(s.IBMVPCCluster.Name, *publicGatewayDetails.CRN)
	if err != nil {
		return nil, err
	}

	return publicGatewayDetails, nil
}

// ReconcileSubnets reconciles the VPC Subnet(s).
func (s *VPCClusterScope) ReconcileSubnets() (bool, error) {
	var subnets []infrav1beta2.Subnet
	var err error
	// If no ControlPlane Subnets were supplied, we default to create one in each zone.
	if s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets == nil || len(s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets) == 0 {
		subnets, err = s.buildSubnetsForZones()
		if err != nil {
			return false, fmt.Errorf("error failed building control plane subnets: %w", err)
		}
	} else {
		subnets = s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets
	}

	// Reconcile Control Plane subnets
	requeue := false
	for _, subnet := range subnets {
		if requiresRequeue, err := s.reconcileSubnet(subnet, true); err != nil {
			return false, fmt.Errorf("error failed reconciling control plane subnet: %w", err)
		} else if requiresRequeue {
			// If the reconcile of the subnet requires further reconciliation, plan to requeue entire ReconcileSubnets call, but attempt to further reconcile additional Subnets (attempt parallel subnet reconciliation)
			requeue = true
		}
	}

	// If no Worker subnets were supplied, attempt to create one in each zone.
	if s.IBMVPCCluster.Spec.Network.WorkerSubnets == nil || len(s.IBMVPCCluster.Spec.Network.WorkerSubnets) == 0 {
		// If neither Control Plane nor Worker subnets were supplied, we rely on both Planes using the same subnet per zone, and we will re-reconcile those subnets below, for IBMVPCCluster Status updates
		if len(s.IBMVPCCluster.Spec.Network.ControlPlaneSubnets) != 0 {
			subnets, err = s.buildSubnetsForZones()
			if err != nil {
				return false, fmt.Errorf("error failed building worker subnets: %w", err)
			}
		}
	} else {
		subnets = s.IBMVPCCluster.Spec.Network.WorkerSubnets
	}

	// Reconcile Worker subnets
	for _, subnet := range subnets {
		if requiresRequeue, err := s.reconcileSubnet(subnet, false); err != nil {
			return false, fmt.Errorf("error failed reconciling worker subnet: %w", err)
		} else if requiresRequeue {
			// If the reconcile of the subnet requires further reconciliation, plan to requeue entire ReconcileSubnets call, but attempt to further reconcile additional Subnets (attempt parallel subnet reconciliation)
			requeue = true
		}
	}

	// Return whether or not one or more subnets required further reconciling after attempting to process all Control Plane and Worker subnets.
	return requeue, nil
}

func (s *VPCClusterScope) buildSubnetsForZones() ([]infrav1beta2.Subnet, error) {
	subnets := make([]infrav1beta2.Subnet, 0)
	zones, err := s.VPCClient.GetZonesByRegion(s.IBMVPCCluster.Spec.Region)
	if err != nil {
		return subnets, err
	}
	if len(zones) == 0 {
		return subnets, fmt.Errorf("error getting subnet zones, no zones found")
	}
	for _, zone := range zones {
		name := fmt.Sprintf("%s-%s", *s.GetServiceName(infrav1beta2.ResourceTypeSubnet), zone)
		zonePtr := ptr.To(zone)
		subnets = append(subnets, infrav1beta2.Subnet{
			Name: ptr.To(name),
			Zone: zonePtr,
		})
	}
	return subnets, nil
}

// reconcileSubnet will attempt to find the existing subnet, or create it if necessary.
// The logic can handle either Control Plane or Worker subnets, but must distinguish between them for Status updates.
func (s *VPCClusterScope) reconcileSubnet(subnet infrav1beta2.Subnet, isControlPlane bool) (bool, error) { //nolint: gocyclo
	var subnetID *string
	// If subnet already has an ID defined, use that for lookup
	if subnet.ID != nil {
		subnetID = subnet.ID
	} else {
		if subnet.Name == nil {
			return false, fmt.Errorf("error subnet has no name or id")
		}
		subnetDetails, err := s.VPCClient.GetVPCSubnetByName(*subnet.Name)
		if err != nil {
			return false, err
		}
		if subnetDetails != nil {
			subnetID = subnetDetails.ID
		}
	}

	if subnetID != nil {
		// Check Cluster Status for the subnet
		if s.IBMVPCCluster.Status.NetworkStatus != nil {
			if isControlPlane {
				// If the subnet is found and the status is already marked Ready, we can shortcut reconcile logic here.
				if status, ok := s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets[*subnet.Name]; ok && status.Ready {
					return false, nil
				}
			} else {
				// If the subnet is found and the status is already marked Ready, we can shortcut reconcile logic here.
				if status, ok := s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets[*subnet.Name]; ok && status.Ready {
					return false, nil
				}
			}
		}

		// Otherwise, if we have a subnet ID, attempt lookup to confirm it exists
		options := &vpcv1.GetSubnetOptions{
			ID: subnetID,
		}
		subnetDetails, _, err := s.VPCClient.GetSubnet(options)
		if err != nil {
			return false, err
		}
		if subnetDetails == nil {
			return false, fmt.Errorf("error failed to get subnet with id %s", *subnetID)
		}
		s.Info("Found Subnet with provided id")

		requeue := true
		if subnetDetails.Status != nil && *subnetDetails.Status == string(vpcv1.SubnetStatusAvailableConst) {
			requeue = false
		}
		// If lookup didn't fail and returned details, we assume it is ready (no further reconciliation necessary). Update the subnet ID for future lookups.
		resourceStatus := infrav1beta2.VPCResourceStatus{
			ID:   *subnetID,
			Name: *subnet.Name,
			// Ready status will be invert of the need to requeue
			Ready: !requeue,
		}
		if isControlPlane {
			s.SetVPCResourceStatus(infrav1beta2.ResourceTypeControlPlaneSubnet, resourceStatus)
		} else {
			s.SetVPCResourceStatus(infrav1beta2.ResourceTypeComputeSubnet, resourceStatus)
		}
		return requeue, nil
	}

	// Since we don't have a subnet Id or couldn't find one, we expect the Subnet doesn't exist yet and we need to create it.
	s.Info("creating subnet", "name", subnet.Name)
	subnetDetails, err := s.createSubnet(subnet)
	if err != nil {
		s.Error(err, "error creating subnet", "name", subnet.Name)
		return false, err
	}
	s.Info("Successfully created subnet", "id", subnetID)

	// Update status with subnet ID for the proper Plane subnet map
	subnetResourceStatus := &infrav1beta2.VPCResourceStatus{
		ID:    *subnetDetails.ID,
		Ready: false,
	}
	if isControlPlane {
		if s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		s.IBMVPCCluster.Status.NetworkStatus.ControlPlaneSubnets[*subnetDetails.ID] = subnetResourceStatus
	} else {
		if s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		s.IBMVPCCluster.Status.NetworkStatus.WorkerSubnets[*subnetDetails.ID] = subnetResourceStatus
	}

	// Recommend we requeue reconciliation after subnet was successfully created
	return true, nil
}

// createSubnet creates a new VPC subnet.
func (s *VPCClusterScope) createSubnet(subnet infrav1beta2.Subnet) (*vpcv1.Subnet, error) {
	// Created resources should be placed in the cluster Resource Group (not Network, if it exists)
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		s.Error(err, "error fetching resource group id for subnet creation")
		return nil, fmt.Errorf("error fetching resource group id for subnet creation")
	} else if resourceGroupID == "" {
		s.Info("failed to create vpc subnet, failed to fetch resource group id")
		return nil, fmt.Errorf("error getting resource group id for resource group %v", s.ResourceGroup())
	}

	vpcID, err := s.GetVPCID()
	if err != nil {
		s.Error(err, "failed to create subnet, failed to fetch vpc id")
		return nil, fmt.Errorf("error getting vpc id for subnet creation: %w", err)
	}

	if subnet.Zone == nil {
		s.Info("subnet zone is not defined", "name", subnet.Name)
		return nil, fmt.Errorf("error subnet zone must be defined for subnet %s", *subnet.Name)
	}

	// NOTE(cjschaef): We likely will want to add support to use custom Address Prefixes
	// For now, we rely on the API to assign us prefixes, as we request via IP count
	var ipCount int64 = 256
	// We currnetly only support IPv4
	ipVersion := "ipv4"

	// Find or create a Public Gateway in this zone for the subnet, only one Public Gateway is required for each zone, for this cluster.
	// NOTE(cjschaef): We may wish to add support to not attach Public Gateways to subnets.
	publicGateway, err := s.findOrCreatePublicGateway(*subnet.Zone)
	if err != nil {
		return nil, err
	}

	options := &vpcv1.CreateSubnetOptions{}
	options.SetSubnetPrototype(&vpcv1.SubnetPrototype{
		IPVersion:             ptr.To(ipVersion),
		TotalIpv4AddressCount: ptr.To(ipCount),
		Name:                  subnet.Name,
		VPC: &vpcv1.VPCIdentity{
			ID: vpcID,
		},
		Zone: &vpcv1.ZoneIdentity{
			Name: subnet.Zone,
		},
		ResourceGroup: &vpcv1.ResourceGroupIdentity{
			ID: ptr.To(resourceGroupID),
		},
		PublicGateway: &vpcv1.PublicGatewayIdentity{
			ID: publicGateway.ID,
		},
	})

	// Create subnet.
	subnetDetails, _, err := s.VPCClient.CreateSubnet(options)
	if err != nil {
		return nil, err
	}
	if subnetDetails == nil {
		s.Info("error failed creating subnet", "name", subnet.Name)
		return nil, fmt.Errorf("error failed creating subnet")
	} else if subnetDetails.ID == nil {
		s.Info("error failed creating subnet, no ID", "name", subnet.Name)
		return nil, fmt.Errorf("error failed creating subnet, no ID")
	} else if subnetDetails.CRN == nil {
		s.Info("error failed creating subnet, no CRN", "name", subnet.Name)
		return nil, fmt.Errorf("error failed creating subnet, no CRN")
	}

	// Add a tag to the subnet for the cluster
	err = s.TagResource(s.IBMVPCCluster.Name, *subnetDetails.CRN)
	if err != nil {
		return nil, err
	}

	return subnetDetails, nil
}

// ReconcileSecurityGroups will attempt to reconcile the defined SecurityGroups and their SecurityGroupRules. Our best option is to perform a first set of passes, creating all the SecurityGroups first, then reconcile the SecurityGroupRules after that, as the SecuirtyGroupRules could be dependent on an IBM Cloud Security Group that must be created first.
func (s *VPCClusterScope) ReconcileSecurityGroups() (bool, error) {
	// If no Security Groups were supplied, we have nothing to do.
	if s.IBMVPCCluster.Spec.Network.SecurityGroups == nil || len(s.IBMVPCCluster.Spec.Network.SecurityGroups) == 0 {
		return false, nil
	}

	// Reconcile each Security Group first, process rules later.
	requeue := false
	for _, securityGroup := range s.IBMVPCCluster.Spec.Network.SecurityGroups {
		if requiresRequeue, err := s.reconcileSecurityGroup(securityGroup); err != nil {
			return false, fmt.Errorf("error failed reonciling security groups: %w", err)
		} else if requiresRequeue {
			requeue = true
		}
	}

	// If one or more Security Groups requires a requeue of reconciliation, let's do that now, and process the Security Group Rules after all Security Groups are reconciled.
	if requeue {
		return true, nil
	}

	// Reconcile each Security Groups's Rules
	requeue = false
	for _, securityGroup := range s.IBMVPCCluster.Spec.Network.SecurityGroups {
		if requiresRequeue, err := s.reconcileSecurityGroupRules(securityGroup); err != nil {
			return false, fmt.Errorf("error failed reconciling security group rules: %w", err)
		} else if requiresRequeue {
			requeue = true
		}
	}

	if requeue {
		return true, nil
	}

	// All Security Groups and Security Group Rules have been reconciled with no requeue's required
	return false, nil
}

// reconcileSecurityGroup will attempt to reconcile a defined SecurityGroup. By design, we confirm the IBM Cloud Security Group exists first, before attempting to reconcile the defined SecurityGroupRules. We return early if the IBM Cloud Security Group did not exist or needed to be created, to return in a followup pass to create the SecurityGroup's Rules.
func (s *VPCClusterScope) reconcileSecurityGroup(securityGroup infrav1beta2.VPCSecurityGroup) (bool, error) {
	var securityGroupID *string
	// If Security Group already has an ID defined, use that for lookup
	if securityGroup.ID != nil {
		securityGroupID = securityGroup.ID
	} else {
		if securityGroup.Name == nil {
			return false, fmt.Errorf("error securityGroup has no name or id")
		}
		// Check the Status if an ID is already available for the Security Group
		if s.IBMVPCCluster.Status.NetworkStatus != nil && s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups != nil {
			if id, ok := s.IBMVPCCluster.Status.NetworkStatus.SecurityGroups[*securityGroup.Name]; ok {
				securityGroupID = &id.ID
			}
		}

		// Otherwise, attempt to lookup the ID by name
		if securityGroupDetails, err := s.VPCClient.GetSecurityGroupByName(*securityGroup.Name); err != nil {
			// If the Security Group was not found, we expect it doesn't exist yet, otherwise result in an error
			if _, ok := err.(*vpc.SecurityGroupByNameNotFound); !ok {
				return false, fmt.Errorf("error failed lookup of security group by name: %w", err)
			}
		} else if securityGroupDetails != nil && securityGroupDetails.ID != nil {
			securityGroupID = securityGroupDetails.ID
		}
	}

	// If we have an ID for the SecurityGroup, we can check the status
	if securityGroupID != nil {
		if securityGroupDetails, _, err := s.VPCClient.GetSecurityGroup(&vpcv1.GetSecurityGroupOptions{
			ID: securityGroupID,
		}); err != nil {
			return false, fmt.Errorf("error failed lookup of security group: %w", err)
		} else if securityGroupDetails == nil {
			// The Security Group cannot be found by ID, it was removed or didn't exist
			// TODO(cjschaef): We may wish to clear the ID's to get a new Security Group created, but for now we return an error
			return false, fmt.Errorf("error could not find security group with id=%s", *securityGroupID)
		}

		// Security Groups do not have a status, so we assume if it exists, it is ready
		s.SetVPCResourceStatus(infrav1beta2.ResourceTypeSecurityGroup, infrav1beta2.VPCResourceStatus{
			ID:    *securityGroupID,
			Ready: true,
		})
		return false, nil
	}

	// If we don't have an ID at this point, we assume we need to create the Security Group
	vpcID, err := s.GetVPCID()
	if err != nil {
		return false, fmt.Errorf("error retrieving vpc id for security group creation")
	}
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		return false, fmt.Errorf("error retrieving resource id for security group creation")
	}
	createOptions := &vpcv1.CreateSecurityGroupOptions{
		Name: securityGroup.Name,
		VPC: &vpcv1.VPCIdentityByID{
			ID: vpcID,
		},
		ResourceGroup: &vpcv1.ResourceGroupIdentityByID{
			ID: ptr.To(resourceGroupID),
		},
	}
	securityGroupDetails, _, err := s.VPCClient.CreateSecurityGroup(createOptions)
	if err != nil {
		s.Error(err, "error creating security group", "name", securityGroup.Name)
		return false, err
	}
	if securityGroupDetails == nil {
		s.Info("error failed creating security group", "name", securityGroup.Name)
		return false, fmt.Errorf("error failed creating security group")
	} else if securityGroupDetails.ID == nil {
		s.Info("error failed creating security group, no ID", "name", securityGroup.Name)
		return false, fmt.Errorf("error failed creating security group, no ID")
	} else if securityGroupDetails.CRN == nil {
		s.Info("error new security group CRN missing, no CRN", "name", securityGroup.Name)
		return false, fmt.Errorf("error failure creating security group, no CRN")
	}

	// Security Groups do not have a status, we could set the status as ready at this point, but for now will trigger a requeue and set status as not ready
	s.SetVPCResourceStatus(infrav1beta2.ResourceTypeSecurityGroup, infrav1beta2.VPCResourceStatus{
		ID:    *securityGroupDetails.ID,
		Ready: false,
	})

	// Add a tag to the Security Group for the cluster
	err = s.TagResource(s.IBMVPCCluster.Name, *securityGroupDetails.CRN)
	if err != nil {
		return false, err
	}

	return true, nil
}

// reconcile SecurityGroupRules will attempt to reconcile the set of defined SecurityGroupRules for a SecurityGroup, one Rule at a time. Each defined Rule can contain multiple remotes, requiring a unique IBM Cloud Security Group Rule, based on the expected traffic direction, inbound (Source) or outbound (Destination).
func (s *VPCClusterScope) reconcileSecurityGroupRules(securityGroup infrav1beta2.VPCSecurityGroup) (bool, error) {
	// We assume that the securityGroup exists in Status, if it doesn't then it should be re-reconciled
	securityGroupID := s.getSecurityGroupID(securityGroup)
	if securityGroupID == nil {
		return true, nil
	}

	// If the SecurityGroup has no rules, we have nothing more to do for this Security Group
	if len(securityGroup.Rules) == 0 {
		return false, nil
	}

	// Reconcile each SecurityGroupRule in the SecurityGroup
	requeue := false
	for _, securityGroupRule := range securityGroup.Rules {
		if requiresRequeue, err := s.reconcileSecurityGroupRule(*securityGroupID, *securityGroupRule); err != nil {
			return false, err
		} else if requiresRequeue {
			requeue = true
		}
	}

	return requeue, nil
}

// reconcileSecurityGroupRule will attempt to reconcile a defined SecurityGroupRule, with one or more Remotes, for a SecurityGroup. If the IBM Cloud Security Group contains no Rules, we simply attempt to create the defined Rule (via the Remote(s) provided).
func (s *VPCClusterScope) reconcileSecurityGroupRule(securityGroupID string, securityGroupRule infrav1beta2.VPCSecurityGroupRule) (bool, error) {
	existingSecurityGroupRuleIntfs, _, err := s.VPCClient.ListSecurityGroupRules(&vpcv1.ListSecurityGroupRulesOptions{
		SecurityGroupID: ptr.To(securityGroupID),
	})
	if err != nil {
		return false, fmt.Errorf("error failed listing security group rules during reconcile of security group id=%s: %w", securityGroupID, err)
	}

	// If the Security Group has no Rules at all, we simply create the Rule
	if existingSecurityGroupRuleIntfs == nil || existingSecurityGroupRuleIntfs.Rules == nil || len(existingSecurityGroupRuleIntfs.Rules) == 0 {
		s.Info("Creating security group rules for security group id=%s", securityGroupID)
		err := s.createSecurityGroupRuleAllRemotes(securityGroupID, securityGroupRule)
		if err != nil {
			return false, err
		}
		s.Info("Created security group rules")

		// Security Group Rules do not have a Status, so we likely don't need to requeue, but for now, will requeue to verify the Security Group Rules
		return true, nil
	}

	// Validate the Security Group Rule(s) exist or create
	if exists, err := s.findOrCreateSecurityGroupRule(securityGroupID, securityGroupRule, existingSecurityGroupRuleIntfs); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}

	// Security Group Rules do not have a Status, so we likely don't need to requeue, but for now, will requeue to verify the Security Group Rules
	return true, nil
}

// findOrCreateSecurityGroupRule will attempt to match up the SecurityGroupRule's Remote(s) (multiple Remotes can be supplied per Rule definition), and will create any missing IBM Cloud Security Group Rules based on the SecurityGroupRule and Remote(s). Remotes are defined either by a Destination (outbound) or a Source (inbound), which defines the type of IBM Cloud Security Group Rule that should exist or be created.
func (s *VPCClusterScope) findOrCreateSecurityGroupRule(securityGroupID string, securityGroupRule infrav1beta2.VPCSecurityGroupRule, existingSecurityGroupRules *vpcv1.SecurityGroupRuleCollection) (bool, error) { //nolint: gocyclo
	// Use either the SecurityGroupRule.Destination or SecurityGroupRule.Source for further details based on SecurityGroupRule.Direction
	var securityGroupRulePrototype infrav1beta2.VPCSecurityGroupRulePrototype
	switch securityGroupRule.Direction {
	case infrav1beta2.VPCSecurityGroupRuleDirectionInbound:
		securityGroupRulePrototype = *securityGroupRule.Source
	case infrav1beta2.VPCSecurityGroupRuleDirectionOutbound:
		securityGroupRulePrototype = *securityGroupRule.Destination
	default:
		return false, fmt.Errorf("error unsupported SecurityGroupRuleDirection defined")
	}

	// Each defined SecurityGroupRule can have multiple Remotes specified, each signifying a separate Security Group Rule (with the same Action, Direction, etc.)
	allMatch := true
	for _, remote := range securityGroupRulePrototype.Remotes {
		remoteMatch := false
		for _, existingRuleIntf := range existingSecurityGroupRules.Rules {
			// Perform analysis of the existingRuleIntf, based on its Protocol type, further analysis is performed based on remaining attributes to find if the specific Rule and Remote match
			switch reflect.TypeOf(existingRuleIntf).String() {
			case infrav1beta2.VPCSecurityGroupRuleProtocolAllType:
				// If our Remote doesn't define all Protocols, we don't need further checks, move on to next Rule
				if securityGroupRulePrototype.Protocol != infrav1beta2.VPCSecurityGroupRuleProtocolAll {
					continue
				}
				existingRule := existingRuleIntf.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolAll)
				// If the Remote doesn't have the same Direction as the Rule, no further checks are necessary
				if securityGroupRule.Direction != infrav1beta2.VPCSecurityGroupRuleDirection(*existingRule.Direction) {
					continue
				}
				if found, err := s.checkSecurityGroupRuleProtocolAll(securityGroupRulePrototype, remote, existingRule); err != nil {
					return false, err
				} else if found {
					// If we found the matching IBM Cloud Security Group Rule for the defined SecurityGroupRule and Remote, we can stop checking IBM Cloud Security Group Rules for this remote and move onto the next remote.
					// The expectation is that only one IBM Cloud Security Group Rule will match, but if at least one matches the defined SecurityGroupRule, that is sufficient.
					remoteMatch = true
					break
				}
			case infrav1beta2.VPCSecurityGroupRuleProtocolIcmpType:
				// If our Remote doesn't define ICMP Protocol, we don't need further checks, move on to next Rule
				if securityGroupRulePrototype.Protocol != infrav1beta2.VPCSecurityGroupRuleProtocolIcmp {
					continue
				}
				existingRule := existingRuleIntf.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolIcmp)
				// If the Remote doesn't have the same Direction as the Rule, no further checks are necessary
				if securityGroupRule.Direction != infrav1beta2.VPCSecurityGroupRuleDirection(*existingRule.Direction) {
					continue
				}
				if found, err := s.checkSecurityGroupRuleProtocolIcmp(securityGroupRulePrototype, remote, existingRule); err != nil {
					return false, err
				} else if found {
					// If we found the matching IBM Cloud Security Group Rule for the defined SecurityGroupRule and Remote, we can stop checking IBM Cloud Security Group Rules for this remote and move onto the next remote.
					remoteMatch = true
					break
				}
			case infrav1beta2.VPCSecurityGroupRuleProtocolTcpudpType:
				// If our Remote doesn't define TCP/UDP Protocol, we don't need further checks, move on to next Rule
				if securityGroupRulePrototype.Protocol != infrav1beta2.VPCSecurityGroupRuleProtocolTCP && securityGroupRulePrototype.Protocol != infrav1beta2.VPCSecurityGroupRuleProtocolUDP {
					continue
				}
				existingRule := existingRuleIntf.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolTcpudp)
				// If the Remote doesn't have the same Direction as the Rule, no further checks are necessary
				if securityGroupRule.Direction != infrav1beta2.VPCSecurityGroupRuleDirection(*existingRule.Direction) {
					continue
				}
				if found, err := s.checkSecurityGroupRuleProtocolTcpudp(securityGroupRulePrototype, remote, existingRule); err != nil {
					return false, err
				} else if found {
					// If we found the matching IBM Cloud Security Group Rule for the defined SecurityGroupRule and Remote, we can stop checking IBM Cloud Security Group Rules for this remote and move onto the next remote.
					remoteMatch = true
					break
				}
			}
		}

		// If we did not find a matching SecurityGroupRule for this defined Remote, create one now and expect to requeue
		if !remoteMatch {
			err := s.createSecurityGroupRule(securityGroupID, securityGroupRule, remote)
			if err != nil {
				return false, err
			}
			allMatch = false
		}
	}
	return allMatch, nil
}

// checkSecurityGroupRuleProtocolAll analyzes an IBM Cloud Security Group Rule designated for 'all' protocols, to verify if the supplied Rule and Remote match the attributes from the existing 'ProtocolAll' Rule.
func (s *VPCClusterScope) checkSecurityGroupRuleProtocolAll(_ infrav1beta2.VPCSecurityGroupRulePrototype, securityGroupRuleRemote infrav1beta2.VPCSecurityGroupRuleRemote, existingRule *vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolAll) (bool, error) {
	if exists, err := s.checkSecurityGroupRulePrototypeRemote(securityGroupRuleRemote, existingRule.Remote); err != nil {
		return false, err
	} else if exists {
		return true, nil
	}
	return false, nil
}

// checkSecurityGroupRuleProtocolIcmp analyzes an IBM Cloud Security Group Rule designated for 'icmp' protocol, to verify if the supplied Rule and Remote match the attributes from the existing 'ProtocolIcmp' Rule.
func (s *VPCClusterScope) checkSecurityGroupRuleProtocolIcmp(securityGroupRulePrototype infrav1beta2.VPCSecurityGroupRulePrototype, securityGroupRuleRemote infrav1beta2.VPCSecurityGroupRuleRemote, existingRule *vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolIcmp) (bool, error) {
	if exists, err := s.checkSecurityGroupRulePrototypeRemote(securityGroupRuleRemote, existingRule.Remote); err != nil {
		return false, err
	} else if !exists {
		return false, nil
	}
	// If ICMPCode is set, then ICMPType must also be set, via kubebuilder specifications
	if securityGroupRulePrototype.ICMPCode != nil && securityGroupRulePrototype.ICMPType != nil {
		// If the existingRule has a Code and Type and they are both equal to the securityGroupRulePrototype's ICMPType and ICMPCode, the existingRule matches our definition for ICMP in securityGroupRulePrototype.
		if existingRule.Code != nil && existingRule.Type != nil {
			if *securityGroupRulePrototype.ICMPCode == *existingRule.Code && *securityGroupRulePrototype.ICMPType == *existingRule.Type {
				return true, nil
			}
		}
	}
	return false, nil
}

// checkSecurityGroupRuleProtocolTcpudp analyzes an IBM Cloud Security Group Rule designated for either 'tcp' or 'udp' protocols, to verify if the supplied Rule and Remote match the attributes from the existing 'ProtocolTcpudp' Rule.
func (s *VPCClusterScope) checkSecurityGroupRuleProtocolTcpudp(securityGroupRulePrototype infrav1beta2.VPCSecurityGroupRulePrototype, securityGroupRuleRemote infrav1beta2.VPCSecurityGroupRuleRemote, existingRule *vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolTcpudp) (bool, error) {
	// Check the protocol next, either TCP or UDP, to verify it matches
	if securityGroupRulePrototype.Protocol != infrav1beta2.VPCSecurityGroupRuleProtocol(*existingRule.Protocol) {
		return false, nil
	}

	if exists, err := s.checkSecurityGroupRulePrototypeRemote(securityGroupRuleRemote, existingRule.Remote); err != nil {
		return false, err
	} else if exists {
		// If PortRange is set, verify whether the MinimumPort and MaximumPort match the existingRule's values, if they are set.
		if securityGroupRulePrototype.PortRange != nil {
			if existingRule.PortMin != nil && securityGroupRulePrototype.PortRange.MinimumPort == *existingRule.PortMin && existingRule.PortMax != nil && securityGroupRulePrototype.PortRange.MaximumPort == *existingRule.PortMax {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *VPCClusterScope) checkSecurityGroupRulePrototypeRemote(securityGroupRuleRemote infrav1beta2.VPCSecurityGroupRuleRemote, existingRemote vpcv1.SecurityGroupRuleRemoteIntf) (bool, error) { //nolint: gocyclo
	// NOTE(cjschaef): We only currently monitor Remote, not Local, as we don't support defining Local in SecurityGroup/SecurityGroupRule.
	switch reflect.TypeOf(existingRemote).String() {
	case infrav1beta2.VPCSecurityGroupRuleRemoteCIDRType:
		if securityGroupRuleRemote.RemoteType == infrav1beta2.VPCSecurityGroupRuleRemoteTypeCIDR {
			cidrRule := existingRemote.(*vpcv1.SecurityGroupRuleRemoteCIDR)
			subnetDetails, err := s.VPCClient.GetVPCSubnetByName(*securityGroupRuleRemote.SecurityGroupName)
			if err != nil {
				return false, fmt.Errorf("error failed getting subnet by name for security group rule: %w", err)
			} else if subnetDetails == nil {
				return false, fmt.Errorf("error failed getting subnet by name for security group rule")
			} else if subnetDetails.Ipv4CIDRBlock == nil {
				return false, fmt.Errorf("error failed getting subnet by name for security group rule, no CIDRBlock")
			}
			if *subnetDetails.Ipv4CIDRBlock == *cidrRule.CIDRBlock {
				return true, nil
			}
		}
	case infrav1beta2.VPCSecurityGroupRuleRemoteIPType:
		ipRule := existingRemote.(*vpcv1.SecurityGroupRuleRemoteIP)
		switch securityGroupRuleRemote.RemoteType {
		case infrav1beta2.VPCSecurityGroupRuleRemoteTypeAddress:
			if *securityGroupRuleRemote.Address == *ipRule.Address {
				return true, nil
			}
		case infrav1beta2.VPCSecurityGroupRuleRemoteTypeAny:
			if *ipRule.Address == infrav1beta2.CIDRBlockAny {
				return true, nil
			}
		}
	case infrav1beta2.VPCSecurityGroupRuleRemoteSecurityGroupReferenceType:
		if securityGroupRuleRemote.RemoteType == infrav1beta2.VPCSecurityGroupRuleRemoteTypeSG {
			sgRule := existingRemote.(*vpcv1.SecurityGroupRuleRemoteSecurityGroupReference)
			// We can compare the SecurityGroup details from the securityGroupRemote and SecurityGroupRuleRemoteSecurityGroupReference, if those values are available
			// Option #1. We can compare the Security Group Name (name is manditory for securityGroupRemote)
			// Option #2. We can compare the Security Group ID (may already have securityGroupRemote ID)
			// Option #3. We can compare the Security Group CRN (need ot lookup the CRN for securityGroupRemote)

			// Option #1: If the SecurityGroupRuleRemoteSecurityGroupReference has a name assigned, we can shortcut and simply check that
			if sgRule.Name != nil && *securityGroupRuleRemote.SecurityGroupName == *sgRule.Name {
				return true, nil
			}
			// Try to get the Security Group Id for quick lookup (from NetworkStatus)
			var securityGroupDetails *vpcv1.SecurityGroup
			var err error
			if securityGroupID := s.getSecurityGroupIDFromStatus(*securityGroupRuleRemote.SecurityGroupName); securityGroupID != nil {
				// Option #2: If the SecurityGroupRuleRemoteSecurityGroupReference has an ID assigned, we can shortcut and simply check that
				if sgRule.ID != nil && *securityGroupID == *sgRule.ID {
					return true, nil
				}
				securityGroupDetails, _, err = s.VPCClient.GetSecurityGroup(&vpcv1.GetSecurityGroupOptions{
					ID: securityGroupID,
				})
			} else {
				securityGroupDetails, err = s.VPCClient.GetSecurityGroupByName(*securityGroupRuleRemote.SecurityGroupName)
			}
			if err != nil {
				return false, fmt.Errorf("error failed getting security group by name for security group rule: %w", err)
			} else if securityGroupDetails == nil {
				return false, fmt.Errorf("error failed getting security group by name for security group rule")
			} else if securityGroupDetails.CRN == nil {
				return false, fmt.Errorf("error failed getting security group by name for security group rule, no CRN")
			}
			// Option #3: We check the SecurityGroupRuleRemoteSecurityGroupReference's CRN, if the Name and ID were not available
			if *securityGroupDetails.CRN == *sgRule.CRN {
				return true, nil
			}
		}
	default:
		if securityGroupRuleRemote.RemoteType == infrav1beta2.VPCSecurityGroupRuleRemoteTypeAny {
			// TODO(cjschaef): determine what to do here, perhaps (??) the following:
			return true, nil
		}
	}
	return false, nil
}

// createSecurityGroupRuleAllRemotes will create one or more IBM Cloud Security Group Rules for a specific SecurityGroup, based on the provided SecurityGroupRule and Remotes defined in the SecurityGroupRule definition (one or more Remotes can be defined per SecurityGroupRule definition).
func (s *VPCClusterScope) createSecurityGroupRuleAllRemotes(securityGroupID string, securityGroupRule infrav1beta2.VPCSecurityGroupRule) error {
	var remotes []infrav1beta2.VPCSecurityGroupRuleRemote
	switch securityGroupRule.Direction {
	case infrav1beta2.VPCSecurityGroupRuleDirectionInbound:
		remotes = securityGroupRule.Source.Remotes
	case infrav1beta2.VPCSecurityGroupRuleDirectionOutbound:
		remotes = securityGroupRule.Destination.Remotes
	}
	for _, remote := range remotes {
		err := s.createSecurityGroupRule(securityGroupID, securityGroupRule, remote)
		if err != nil {
			return fmt.Errorf("error failed creating security group rule: %w", err)
		}
	}

	return nil
}

// createSecurityGroupRule will create a new IBM Cloud Security Group Rule for a specific Security Group, based on the provided SecurityGroupRule and Remote definitions.
func (s *VPCClusterScope) createSecurityGroupRule(securityGroupID string, securityGroupRule infrav1beta2.VPCSecurityGroupRule, remote infrav1beta2.VPCSecurityGroupRuleRemote) error {
	options := &vpcv1.CreateSecurityGroupRuleOptions{
		SecurityGroupID: &securityGroupID,
	}
	// Setup variables to use for logging details on the resulting IBM Cloud Security Group Rule creation options
	var securityGroupRulePrototype *infrav1beta2.VPCSecurityGroupRulePrototype
	if securityGroupRule.Direction == infrav1beta2.VPCSecurityGroupRuleDirectionInbound {
		securityGroupRulePrototype = securityGroupRule.Source
	} else {
		securityGroupRulePrototype = securityGroupRule.Destination
	}
	prototypeRemote, err := s.createSecurityGroupRuleRemote(remote)
	if err != nil {
		return err
	}
	switch securityGroupRulePrototype.Protocol {
	case infrav1beta2.VPCSecurityGroupRuleProtocolAll:
		prototype := &vpcv1.SecurityGroupRulePrototypeSecurityGroupRuleProtocolAll{
			Direction: ptr.To(string(securityGroupRule.Direction)),
			Protocol:  ptr.To(string(securityGroupRulePrototype.Protocol)),
			Remote:    prototypeRemote,
		}
		options.SetSecurityGroupRulePrototype(prototype)
	case infrav1beta2.VPCSecurityGroupRuleProtocolIcmp:
		prototype := &vpcv1.SecurityGroupRulePrototypeSecurityGroupRuleProtocolIcmp{
			Direction: ptr.To(string(securityGroupRule.Direction)),
			Protocol:  ptr.To(string(securityGroupRulePrototype.Protocol)),
			Remote:    prototypeRemote,
		}
		// If ICMP Code or Type is specified, both must be, enforced by kubebuilder
		if securityGroupRulePrototype.ICMPCode != nil && securityGroupRulePrototype.ICMPType != nil {
			prototype.Code = securityGroupRulePrototype.ICMPCode
			prototype.Type = securityGroupRulePrototype.ICMPType
		}
		options.SetSecurityGroupRulePrototype(prototype)
	// TCP and UDP use the same Prototype, simply with different Protocols, which is agnostic in code
	case infrav1beta2.VPCSecurityGroupRuleProtocolTCP, infrav1beta2.VPCSecurityGroupRuleProtocolUDP:
		prototype := &vpcv1.SecurityGroupRulePrototypeSecurityGroupRuleProtocolTcpudp{
			Direction: ptr.To(string(securityGroupRule.Direction)),
			Protocol:  ptr.To(string(securityGroupRulePrototype.Protocol)),
			Remote:    prototypeRemote,
		}
		if securityGroupRulePrototype.PortRange != nil {
			prototype.PortMin = ptr.To(securityGroupRulePrototype.PortRange.MinimumPort)
			prototype.PortMax = ptr.To(securityGroupRulePrototype.PortRange.MaximumPort)
		}
		options.SetSecurityGroupRulePrototype(prototype)
	default:
		// This should not be possible, provided the strict kubebuilder enforcements
		return fmt.Errorf("error failed creating security group rule, unknown protocol")
	}

	s.Info("Creating Security Group Rule for Security Group", "id", securityGroupID, "direction", securityGroupRule.Direction, "protocol", securityGroupRulePrototype.Protocol, "prototypeRemote", prototypeRemote)
	securityGroupRuleIntfDetails, _, err := s.VPCClient.CreateSecurityGroupRule(options)
	if err != nil {
		return err
	} else if securityGroupRuleIntfDetails == nil {
		return fmt.Errorf("error failed creating security group rule")
	}

	// Typecast the resulting SecurityGroupRuleIntf, to retrieve the ID for logging
	var ruleID *string
	switch reflect.TypeOf(securityGroupRuleIntfDetails).String() {
	case infrav1beta2.VPCSecurityGroupRuleProtocolAllType:
		rule := securityGroupRuleIntfDetails.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolAll)
		ruleID = rule.ID
	case infrav1beta2.VPCSecurityGroupRuleProtocolIcmpType:
		rule := securityGroupRuleIntfDetails.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolIcmp)
		ruleID = rule.ID
	case infrav1beta2.VPCSecurityGroupRuleProtocolTcpudpType:
		rule := securityGroupRuleIntfDetails.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolTcpudp)
		ruleID = rule.ID
	}
	s.Info("Created Security Group Rule", "id", ruleID)
	return nil
}

// createSecurityGroupRuleRemote will create an IBM Cloud SecurityGroupRuleRemotePrototype, which defines the Remote details for an IBM Cloud Security Group Rule, provided by the SecurityGroupRuleRemote. Lookups of Security Group CRN's, by Name, or Subnet CIDRBlock's, by Name, allows the use of CAPI created resources to be defined in the SecurityGroupRuleRemote, when the CRN or CIDRBlock are unknown (runtime defined).
func (s *VPCClusterScope) createSecurityGroupRuleRemote(remote infrav1beta2.VPCSecurityGroupRuleRemote) (*vpcv1.SecurityGroupRuleRemotePrototype, error) {
	remotePrototype := &vpcv1.SecurityGroupRuleRemotePrototype{}
	switch remote.RemoteType {
	case infrav1beta2.VPCSecurityGroupRuleRemoteTypeAny:
		remotePrototype.CIDRBlock = ptr.To(infrav1beta2.CIDRBlockAny)
	case infrav1beta2.VPCSecurityGroupRuleRemoteTypeCIDR:
		// As we nned the Subnet CIDR block, we have to perform an IBM Cloud API call either way, so simply make the call using the item we know, the Name
		subnetDetails, err := s.VPCClient.GetVPCSubnetByName(*remote.CIDRSubnetName)
		if err != nil {
			return nil, fmt.Errorf("error failed lookup of subnet during security group rule remote creation: %w", err)
		} else if subnetDetails == nil {
			return nil, fmt.Errorf("error failed lookup of subnet during security group rule remote creation")
		} else if subnetDetails.Ipv4CIDRBlock == nil {
			return nil, fmt.Errorf("error failed lookup of subnet during security group rule remote creation, no Ipv4CIDRBlock")
		}
		remotePrototype.CIDRBlock = subnetDetails.Ipv4CIDRBlock
	case infrav1beta2.VPCSecurityGroupRuleRemoteTypeAddress:
		remotePrototype.Address = remote.Address
	case infrav1beta2.VPCSecurityGroupRuleRemoteTypeSG:
		// As we need the Security Group CRN, we have to perform an IBM Cloud API call either way, so simply make the call using the item we know, the Name
		securityGroupDetails, err := s.VPCClient.GetSecurityGroupByName(*remote.SecurityGroupName)
		if err != nil {
			return nil, fmt.Errorf("error failed lookup of security group during security group rule remote creation: %w", err)
		} else if securityGroupDetails == nil {
			return nil, fmt.Errorf("error failed lookup of security group during security group rule remote creation")
		} else if securityGroupDetails.CRN == nil {
			return nil, fmt.Errorf("error failed lookup of security group during security group rule remote creation, no CRN")
		}
		remotePrototype.CRN = securityGroupDetails.CRN
	default:
		// This should not be possible, given the strict kubebuilder enforcements
		return nil, fmt.Errorf("error failed creating security group rule remote")
	}

	return remotePrototype, nil
}

// ReconcileLoadBalancers reconciles Load Balancers.
func (s *VPCClusterScope) ReconcileLoadBalancers() (bool, error) {
	if len(s.IBMVPCCluster.Spec.Network.LoadBalancers) == 0 {
		// We currently don't support any default LB configuration, they must be specified within the Cluster Spec
		return false, fmt.Errorf("error no load balancers specified for cluster")
	}

	for _, loadBalancer := range s.IBMVPCCluster.Spec.Network.LoadBalancers {
		var loadBalancerID *string
		if loadBalancer.ID != nil {
			loadBalancerID = loadBalancer.ID
		} else {
			if loadBalancer.Name == "" {
				return false, fmt.Errorf("error load balancer has no name or id")
			}
			lbDetails, err := s.VPCClient.GetLoadBalancerByName(loadBalancer.Name)
			if err != nil {
				return false, err
			}
			if lbDetails != nil {
				loadBalancerID = lbDetails.ID
			}
		}

		if loadBalancerID != nil {
			// Check Cluster Status for Load Balancer
			if s.IBMVPCCluster.Status.NetworkStatus != nil {
				// If the load balancer is found and the state is active, we can shortcut reconcile logic on this load balancer and move on to the next one.
				if status, ok := s.IBMVPCCluster.Status.NetworkStatus.LoadBalancers[*loadBalancerID]; ok && status.State == infrav1beta2.VPCLoadBalancerStateActive {
					continue
				}
			}
			s.Info("LoadBalancer ID is set, fetching loadbalancer details", "loadbalancerid", *loadBalancerID)
			loadBalancer, _, err := s.VPCClient.GetLoadBalancer(&vpcv1.GetLoadBalancerOptions{
				ID: loadBalancerID,
			})
			if err != nil {
				return false, err
			}

			if requeue := s.checkLoadBalancerStatus(loadBalancer.ProvisioningStatus); requeue {
				return requeue, nil
			}

			loadBalancerStatus := infrav1beta2.VPCLoadBalancerStatus{
				ID:       loadBalancer.ID,
				State:    infrav1beta2.VPCLoadBalancerState(*loadBalancer.ProvisioningStatus),
				Hostname: loadBalancer.Hostname,
			}
			s.SetLoadBalancerStatus(loadBalancerStatus)
			continue
		}
		// check VPC load balancer exist in cloud
		loadBalancerStatus, err := s.checkLoadBalancer(loadBalancer)
		if err != nil {
			return false, err
		}
		if loadBalancerStatus != nil {
			s.SetLoadBalancerStatus(*loadBalancerStatus)
			continue
		}
		// create loadBalancer
		loadBalancerStatus, err = s.createLoadBalancer(loadBalancer)
		if err != nil {
			return false, err
		}
		s.Info("Created VPC load balancer", "id", loadBalancerStatus.ID)
		s.SetLoadBalancerStatus(*loadBalancerStatus)

		// tag

		return true, nil
	}
	return false, nil
}

// checkLoadBalancerStatus checks the state of a VPC load balancer.
// If state is pending, true is returned indicating a requeue for reconciliation.
// In all other cases, it returns false.
func (s *VPCClusterScope) checkLoadBalancerStatus(status *string) bool {
	switch *status {
	case string(infrav1beta2.VPCLoadBalancerStateActive):
		s.Info("VPC load balancer is in active state")
	case string(infrav1beta2.VPCLoadBalancerStateCreatePending):
		s.Info("VPC load balancer is in create pending state")
		return true
	}
	return false
}

// checkLoadBalancer checks loadBalancer in cloud.
func (s *VPCClusterScope) checkLoadBalancer(lb infrav1beta2.VPCLoadBalancerSpec) (*infrav1beta2.VPCLoadBalancerStatus, error) {
	loadBalancer, err := s.VPCClient.GetLoadBalancerByName(lb.Name)
	if err != nil {
		return nil, err
	}
	if loadBalancer == nil {
		return nil, nil
	}
	return &infrav1beta2.VPCLoadBalancerStatus{
		ID:       loadBalancer.ID,
		State:    infrav1beta2.VPCLoadBalancerState(*loadBalancer.ProvisioningStatus),
		Hostname: loadBalancer.Hostname,
	}, nil
}

// createLoadBalancer creates loadBalancer.
func (s *VPCClusterScope) createLoadBalancer(lb infrav1beta2.VPCLoadBalancerSpec) (*infrav1beta2.VPCLoadBalancerStatus, error) {
	options := &vpcv1.CreateLoadBalancerOptions{}
	// TODO(karthik-k-n): consider moving resource group id to clusterscope
	// fetch resource group id
	resourceGroupID, err := s.GetResourceGroupID()
	if err != nil {
		return nil, err
	}
	if resourceGroupID == "" {
		s.Info("failed to create load balancer, failed to fetch resource group id")
		return nil, fmt.Errorf("error getting resource group id for resource group %v, id is empty", s.ResourceGroup())
	}

	var isPublic bool
	if lb.Public != nil && *lb.Public {
		isPublic = true
	}
	options.SetIsPublic(isPublic)
	options.SetName(lb.Name)
	options.SetResourceGroup(&vpcv1.ResourceGroupIdentity{
		ID: &resourceGroupID,
	})

	subnetIDs, err := s.GetSubnetIDs()
	if err != nil {
		return nil, fmt.Errorf("error collecting subnet IDs for load balancer creation")
	} else if subnetIDs == nil {
		return nil, fmt.Errorf("error subnet required for load balancer creation")
	}
	for _, subnetID := range subnetIDs {
		subnet := &vpcv1.SubnetIdentity{
			ID: ptr.To(subnetID),
		}
		options.Subnets = append(options.Subnets, subnet)
	}
	// TODO(cjschaef): Determine if this Pool should be auto generated or required from Spec
	options.SetPools([]vpcv1.LoadBalancerPoolPrototype{
		{
			Algorithm:     core.StringPtr("round_robin"),
			HealthMonitor: &vpcv1.LoadBalancerPoolHealthMonitorPrototype{Delay: core.Int64Ptr(5), MaxRetries: core.Int64Ptr(2), Timeout: core.Int64Ptr(2), Type: core.StringPtr("tcp")},
			// Note: Appending port number to the name, it will be referenced to set target port while adding new pool member
			Name:     core.StringPtr(fmt.Sprintf("%s-pool-%d", lb.Name, s.APIServerPort())),
			Protocol: core.StringPtr("tcp"),
		},
	})

	// TODO(cjschaef): Determine if this Listener should be auto applied or required from Spec
	options.SetListeners([]vpcv1.LoadBalancerListenerPrototypeLoadBalancerContext{
		{
			Protocol: core.StringPtr("tcp"),
			Port:     core.Int64Ptr(int64(s.APIServerPort())),
			DefaultPool: &vpcv1.LoadBalancerPoolIdentityByName{
				Name: core.StringPtr(fmt.Sprintf("%s-pool-%d", lb.Name, s.APIServerPort())),
			},
		},
	})

	if lb.AdditionalListeners != nil {
		for _, additionalListeners := range lb.AdditionalListeners {
			pool := vpcv1.LoadBalancerPoolPrototype{
				Algorithm:     core.StringPtr("round_robin"),
				HealthMonitor: &vpcv1.LoadBalancerPoolHealthMonitorPrototype{Delay: core.Int64Ptr(5), MaxRetries: core.Int64Ptr(2), Timeout: core.Int64Ptr(2), Type: core.StringPtr("tcp")},
				// Note: Appending port number to the name, it will be referenced to set target port while adding new pool member
				Name:     ptr.To(fmt.Sprintf("additional-pool-%d", additionalListeners.Port)),
				Protocol: core.StringPtr("tcp"),
			}
			options.Pools = append(options.Pools, pool)

			listener := vpcv1.LoadBalancerListenerPrototypeLoadBalancerContext{
				Protocol: core.StringPtr("tcp"),
				Port:     core.Int64Ptr(additionalListeners.Port),
				DefaultPool: &vpcv1.LoadBalancerPoolIdentityByName{
					Name: ptr.To(fmt.Sprintf("additional-pool-%d", additionalListeners.Port)),
				},
			}
			options.Listeners = append(options.Listeners, listener)
		}
	}

	loadBalancer, _, err := s.VPCClient.CreateLoadBalancer(options)
	if err != nil {
		return nil, err
	}
	lbState := infrav1beta2.VPCLoadBalancerState(*loadBalancer.ProvisioningStatus)
	return &infrav1beta2.VPCLoadBalancerStatus{
		ID:                loadBalancer.ID,
		State:             lbState,
		Hostname:          loadBalancer.Hostname,
		ControllerCreated: ptr.To(true),
	}, nil
}

/*
// getVPCRegion returns region associated with VPC zone.
func (s *VPCClusterScope) getVPCRegion() *string {
	if s.IBMVPCCluster.Spec.VPC != nil {
		return s.IBMVPCCluster.Spec.VPC.Region
	}
	// if vpc region is not set try to fetch corresponding region from power vs zone
	zone := s.Zone()
	if zone == nil {
		s.Info("powervs zone is not set")
		return nil
	}
	region := endpoints.ConstructRegionFromZone(*zone)
	vpcRegion, err := genUtil.VPCRegionForPowerVSRegion(region)
	if err != nil {
		s.Error(err, fmt.Sprintf("failed to fetch vpc region associated with powervs region %s", region))
		return nil
	}
	return &vpcRegion
}

// fetchVPCCRN returns VPC CRN.
func (s *VPCClusterScope) fetchVPCCRN() (*string, error) {
	vpcDetails, _, err := s.IBMVPCClient.GetVPC(&vpcv1.GetVPCOptions{
		ID: s.GetVPCID(),
	})
	if err != nil {
		return nil, err
	}
	return vpcDetails.CRN, nil
}.
*/

// GetServiceName returns name of given service type from spec or generate a name for it.
func (s *VPCClusterScope) GetServiceName(resourceType infrav1beta2.ResourceType) *string {
	switch resourceType {
	case infrav1beta2.ResourceTypeVPC:
		if s.VPC() == nil || s.VPC().Name == nil {
			return ptr.To(fmt.Sprintf("%s-vpc", s.InfraCluster()))
		}
		return s.VPC().Name
	case infrav1beta2.ResourceTypeSubnet:
		// Return a Generic Subnet name, which can be extended as necessary (for Zone)
		return ptr.To(fmt.Sprintf("%s-subnet", s.IBMVPCCluster.Name))
	case infrav1beta2.ResourceTypePublicGateway:
		return ptr.To(fmt.Sprintf("%s-pgateway", s.IBMVPCCluster.Name))
	default:
		s.Info("unsupported resource type")
	}
	return nil
}

/*
// DeleteLoadBalancer deletes loadBalancer.
func (s *VPCClusterScope) DeleteLoadBalancer() (bool, error) {
	for _, lb := range s.IBMVPCCluster.Status.LoadBalancers {
		if lb.ID == nil || lb.ControllerCreated == nil || !*lb.ControllerCreated {
			continue
		}

		lb, _, err := s.IBMVPCClient.GetLoadBalancer(&vpcv1.GetLoadBalancerOptions{
			ID: lb.ID,
		})

		if err != nil {
			if strings.Contains(err.Error(), "cannot be found") {
				s.Info("VPC load balancer successfully deleted")
				return false, nil
			}
			return false, fmt.Errorf("error fetching the load balancer: %w", err)
		}

		if lb != nil && lb.ProvisioningStatus != nil && *lb.ProvisioningStatus == string(infrav1beta2.VPCLoadBalancerStateDeletePending) {
			s.Info("VPC load balancer is currently being deleted")
			return true, nil
		}

		if _, err = s.IBMVPCClient.DeleteLoadBalancer(&vpcv1.DeleteLoadBalancerOptions{
			ID: lb.ID,
		}); err != nil {
			s.Error(err, "error deleting the load balancer")
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// DeleteVPCSubnet deletes VPC subnet.
func (s *VPCClusterScope) DeleteVPCSubnet() (bool, error) {
	for _, subnet := range s.IBMVPCCluster.Status.VPCSubnet {
		if subnet.ID == nil || subnet.ControllerCreated == nil || !*subnet.ControllerCreated {
			continue
		}

		net, _, err := s.IBMVPCClient.GetSubnet(&vpcv1.GetSubnetOptions{
			ID: subnet.ID,
		})

		if err != nil {
			if strings.Contains(err.Error(), "Subnet not found") {
				s.Info("VPC subnet successfully deleted")
				return false, nil
			}
			return false, fmt.Errorf("error fetching the subnet: %w", err)
		}

		if net != nil && net.Status != nil && *net.Status == string(infrav1beta2.VPCSubnetStateDeleting) {
			return true, nil
		}

		if _, err = s.IBMVPCClient.DeleteSubnet(&vpcv1.DeleteSubnetOptions{
			ID: net.ID,
		}); err != nil {
			return false, fmt.Errorf("error deleting VPC subnet: %w", err)
		}
		return true, nil
	}
	return false, nil
}

// DeleteVPC deletes VPC.
func (s *VPCClusterScope) DeleteVPC() (bool, error) {
	if !s.isResourceCreatedByController(infrav1beta2.ResourceTypeVPC) {
		return false, nil
	}

	if s.IBMVPCCluster.Status.VPC.ID == nil {
		return false, nil
	}

	vpc, _, err := s.IBMVPCClient.GetVPC(&vpcv1.GetVPCOptions{
		ID: s.IBMVPCCluster.Status.VPC.ID,
	})

	if err != nil {
		if strings.Contains(err.Error(), "VPC not found") {
			s.Info("VPC successfully deleted")
			return false, nil
		}
		return false, fmt.Errorf("error fetching the VPC: %w", err)
	}

	if vpc != nil && vpc.Status != nil && *vpc.Status == string(infrav1beta2.VPCStateDeleting) {
		return true, nil
	}

	if _, err = s.IBMVPCClient.DeleteVPC(&vpcv1.DeleteVPCOptions{
		ID: vpc.ID,
	}); err != nil {
		return false, fmt.Errorf("error deleting VPC: %w", err)
	}
	return true, nil
}.
*/

// CheckTagExists checks whether a user Tag already exists.
func (s *VPCClusterScope) CheckTagExists(tagName string) (bool, error) {
	exists, err := s.GlobalTaggingClient.GetTagByName(tagName)
	if err != nil {
		return false, err
	}
	return exists != nil, nil
}

// TagResource will attach a user Tag to a resource.
func (s *VPCClusterScope) TagResource(tagName string, resourceCRN string) error {
	// Verify the Tag we wish to use exists, otherwise create it.
	exists, err := s.CheckTagExists(tagName)
	if err != nil {
		return err
	}
	// Create tag if it doesn't exist.
	if !exists {
		options := &globaltaggingv1.CreateTagOptions{}
		options.SetTagNames([]string{tagName})
		if _, _, err = s.GlobalTaggingClient.CreateTag(options); err != nil {
			return err
		}
	}
	options := &globaltaggingv1.AttachTagOptions{}
	options.SetResources([]globaltaggingv1.Resource{
		{
			ResourceID: ptr.To(resourceCRN),
		},
	})
	options.SetTagName(tagName)
	options.SetTagType(globaltaggingv1.AttachTagOptionsTagTypeUserConst)

	if _, _, err = s.GlobalTaggingClient.AttachTag(options); err != nil {
		return err
	}
	return nil
}

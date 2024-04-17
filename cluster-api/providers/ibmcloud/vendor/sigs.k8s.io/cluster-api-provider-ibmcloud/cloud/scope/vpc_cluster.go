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
	genUtil "sigs.k8s.io/cluster-api-provider-ibmcloud/util"
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

	// if vpc.cluster.x-k8s.io/create-infra=true annotation is not set, only need vpc client.
	if !genUtil.CheckCreateVPCInfraAnnotation(*params.IBMVPCCluster) {
		return &VPCClusterScope{
			Logger:          params.Logger,
			Client:          params.Client,
			patchHelper:     helper,
			Cluster:         params.Cluster,
			IBMVPCCluster:   params.IBMVPCCluster,
			ServiceEndpoint: params.ServiceEndpoint,
			VPCClient:       vpcClient,
		}, nil
	}

	// if vpc.cluster.x-k8s.io/create-infra=true annotation is set, create necessary clients.
	if params.IBMVPCCluster.Spec.NetworkSpec == nil || params.IBMVPCCluster.Spec.Region == "" {
		return nil, fmt.Errorf("error failed to generate vpc client as NetworkSpec info is nil")
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
	rmOptions := resourcemanager.ServiceOptions{
		ResourceManagerV2Options: &resourcemanagerv2.ResourceManagerV2Options{
			Authenticator: auth,
		},
	}
	// Fetch the ResourceManager endpoint.
	rmEndpoint := endpoints.FetchEndpoints(string(endpoints.ResourceManager), params.ServiceEndpoint)
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
	if s.IBMVPCCluster.Spec.NetworkSpec != nil && s.IBMVPCCluster.Spec.NetworkSpec.ResourceGroup != nil {
		return *s.IBMVPCCluster.Spec.NetworkSpec.ResourceGroup
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
		if s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets[resource.Name]; ok {
			subnet.Set(resource)
		} else {
			s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets[resource.Name] = ptr.To(resource)
		}
	default:
		s.Info("unsupported vpc resource type")
	}
}

// NetworkSpec returns the cluster NetworkSpec.
func (s *VPCClusterScope) NetworkSpec() *infrav1beta2.VPCNetworkSpec {
	return s.IBMVPCCluster.Spec.NetworkSpec
}

// VPC returns the cluster VPC information.
func (s *VPCClusterScope) VPC() *infrav1beta2.VPCResource {
	if s.IBMVPCCluster.Spec.NetworkSpec == nil {
		return nil
	}
	return s.IBMVPCCluster.Spec.NetworkSpec.VPC
}

// GetVPCID returns the VPC id.
func (s *VPCClusterScope) GetVPCID() (*string, error) {
	if s.IBMVPCCluster.Status.NetworkStatus != nil && s.IBMVPCCluster.Status.NetworkStatus.VPC != nil {
		return ptr.To(s.IBMVPCCluster.Status.NetworkStatus.VPC.ID), nil
	}
	if s.IBMVPCCluster.Spec.NetworkSpec != nil && s.IBMVPCCluster.Spec.NetworkSpec.VPC != nil {
		if s.IBMVPCCluster.Spec.NetworkSpec.VPC.ID != nil {
			return s.IBMVPCCluster.Spec.NetworkSpec.VPC.ID, nil
		} else if s.IBMVPCCluster.Spec.NetworkSpec.VPC.Name != nil {
			vpc, err := s.VPCClient.GetVPCByName(*s.IBMVPCCluster.Spec.NetworkSpec.VPC.Name)
			if err != nil {
				return nil, err
			}
			// Check if VPC was found and has an ID
			if vpc != nil && vpc.ID != nil {
				// Set VPC ID to shortcut future lookups
				s.IBMVPCCluster.Spec.NetworkSpec.VPC.ID = vpc.ID
				return s.IBMVPCCluster.Spec.NetworkSpec.VPC.ID, nil
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
		if s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets != nil {
			if subnet, ok := s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets[name]; ok {
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
		if s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets != nil {
			for _, subnet := range s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets {
				if _, exists := subnetMap[subnet.ID]; !exists {
					subnetMap[subnet.ID] = true
				}
			}
		}
	} else if s.IBMVPCCluster.Spec.NetworkSpec != nil {
		// Check for Id's in ControlPlaneSubnetSpec, or try Id lookups by name
		if s.IBMVPCCluster.Spec.NetworkSpec.ControlPlaneSubnetsSpec != nil {
			err := checkSubnets(s.IBMVPCCluster.Spec.NetworkSpec.ControlPlaneSubnetsSpec)
			if err != nil {
				return nil, err
			}
		}
		if s.IBMVPCCluster.Spec.NetworkSpec.ComputeSubnetsSpec != nil {
			err := checkSubnets(s.IBMVPCCluster.Spec.NetworkSpec.ComputeSubnetsSpec)
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
	if s.IBMVPCCluster.Spec.NetworkSpec != nil && s.IBMVPCCluster.Spec.NetworkSpec.ResourceGroup != nil {
		// Retrieve the Resource Group based on the name
		resourceGroup, err := s.ResourceManagerClient.GetResourceGroupByName(*s.IBMVPCCluster.Spec.NetworkSpec.ResourceGroup)
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
	// if VPC server id is set means the VPC is already created
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
	// For VPC, we may rely on using the Network's Resource Group ID, if it was provided. Otherwise, we default to the cluster's Resource Group ID.
	resourceGroupID, err := s.GetNetworkResourceGroupID()
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
	if s.IBMVPCCluster.Status.ImageStatus != nil {
		if s.IBMVPCCluster.Status.ImageStatus.ID != "" {
			imageID = ptr.To(s.IBMVPCCluster.Status.ImageStatus.ID)
		} else if s.IBMVPCCluster.Status.ImageStatus.Name != "" {
			image, err := s.VPCClient.GetImageByName(s.IBMVPCCluster.Status.ImageStatus.Name)
			if err != nil {
				return false, err
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
			return false, err
		}
		if image == nil {
			return false, fmt.Errorf("error failed to get vpc custom image with id %s", *imageID)
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
		return false, err
	} else if image == nil {
		return false, fmt.Errorf("error failed to create vpc custom image")
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
				return nil, err
			}
			resourceGroupID = id.ID
		}
	} else {
		// Otherwise, we use the Network Resource Group ID, if it exists, or just the cluster Resource Group ID.
		id, err := s.GetNetworkResourceGroupID()
		if err != nil {
			return nil, err
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
		return nil, err
	} else if fileHRef == nil {
		return nil, fmt.Errorf("error failed to build vpc custom image file href")
	}

	options := &vpcv1.CreateImageOptions{
		ImagePrototype: &vpcv1.ImagePrototype{
			Name: &s.IBMVPCCluster.Spec.Image.Name,
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
		return nil, err
	}
	if err := s.TagResource(s.IBMVPCCluster.Name, *imageDetails.CRN); err != nil {
		return nil, err
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

/*
// ReconcilePublicGateways reconciles the VPC Public Gateway(s).
func (s *VPCClusterScope) ReconcilePublicGateways() (bool, error) {
	// If Public Gateways are not enabled, we have nothing to reconcile
	if s.NetworkSpec().EnablePublicGateways != nil && !*s.NetworkSpec().EnablePublicGateways {
		return false, nil
	}

	// Retrieve the zones covered by any supplied subnets.
	subnetZones, err := s.getSubnetZones()
	if err != nil {
		return false, err
	}

	// If no zones are listed, we expect to default to using all zones in the region.
	if len(subnetZones) == 0 {
		zones, err := s.VPCClient.GetZonesByRegion(s.IBMVPCCluster.Spec.Region)
		if err != nil {
			return false, err
		}
		subnetZones = append(subnetZones, zones...)
	}

	// Now that we should have the expected subnet zones, we can check those covered by any Public Gateways in Status, to confirm if they exist and no additional Public Gateways are needed.
	ready, err := s.validatePublicGatewayStatus(subnetZones)
	if err != nil {
		return false, err
	} else if ready {
		return false, nil
	}

	// Otherwise, get the existing Public Gateways created for this cluster, from Status.
	existingPublicGateways := make(map[string]*infrav1beta2.VPCResourceStatus, 0)
	if s.IBMVPCCluster.Status.NetworkStatus != nil && len(s.IBMVPCCluster.Status.NetworkStatus.PublicGateways) != 0 {
		for k, v := range s.IBMVPCCluster.Status.NetworkStatus.PublicGateways {
			existingPublicGateways[k] = v
		}
	}

	// Get zones (which expect to contain a subnet) which do not have a Public Gateway as of yet.
	missingZones, err := s.getMissingPublicGatewayZones(existingPublicGateways, subnetZones)
	if err != nil {
		return false, err
	}

	// Process remaining zones to check or create a Public Gateway.
	requeue := false
	for _, zone := range missingZones {
		publicGateway, err := s.VPCClient.GetPublicGatewayByZone(zone)
		if err != nil {
			return false, err
		}

		if publicGateway != nil {
			if publicGateway.Status != nil && *publicGateway.Status == vpcv1.PublicGatewayStatusAvailableConst {
				// If the Public Gateway was ready, update its status and move onto next Public Gateway (zone).
				s.SetVPCResourceStatus(infrav1beta2.ResourceTypePublicGateway, infrav1beta2.VPCResourceStatus{
					ID:    *publicGateway.ID,
					Ready: true,
				})
			} else {
				// Otherwise, requeue to give the Public Gateway more time to be ready, move onto next Public Gateway.
				requeue = true
			}
			continue
		}

		// Since a Public Gateway was not found, attempt to create one for the zone.
		publicGateway, err = s.createPublicGateway(zone)
		if err != nil {
			return false, err
		}
		if publicGateway == nil || publicGateway.ID == nil {
			return false, fmt.Errorf("error failure creating public gateway in zone %s", zone)
		}
		s.SetVPCResourceStatus(infrav1beta2.ResourceTypePublicGateway, infrav1beta2.VPCResourceStatus{
			ID:    *publicGateway.ID,
			Ready: false,
		})
	}

	return requeue, nil
}.
*/

// findOrCreatePublicGateway will attempt to find if there is an existing Public Gateway for a specific zone, for the cluster (in cluster's/Network's Resource Group and VPC), or create a new one. Only one Public Gateway is required in each zone, for any subnets in that zone.
func (s *VPCClusterScope) findOrCreatePublicGateway(zone string) (*vpcv1.PublicGateway, error) {
	publicGatewayName := fmt.Sprintf("%s-%s", *s.GetServiceName(infrav1beta2.ResourceTypePublicGateway), zone)
	// We will use the Network Resource Group ID (as we'd expect the Public Gateways for the Network to be in that Resource Group).
	resourceGroupID, err := s.GetNetworkResourceGroupID()
	if err != nil {
		return nil, err
	}
	publicGateway, err := s.VPCClient.GetPublicGatewayByName(publicGatewayName, resourceGroupID)
	if err != nil {
		return nil, err
	}
	// If we found the Public Gateway, with an ID, for the zone, return it.
	// NOTE(cjschaef): We may wish to confirm the PublicGateway, by checking Tags (Global Tagging), but this might be sufficient.
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
	if publicGatewayDetails == nil || publicGatewayDetails.ID == nil {
		return nil, fmt.Errorf("error failed creating public gateway for zone %s", zone)
	}
	return publicGatewayDetails, nil
}

// ReconcileSubnets reconciles the VPC Subnet(s).
func (s *VPCClusterScope) ReconcileSubnets() (bool, error) {
	var subnets []infrav1beta2.Subnet
	var err error
	// If no ControlPlane Subnets were supplied, we default to create one in each zone.
	if len(s.IBMVPCCluster.Spec.NetworkSpec.ControlPlaneSubnetsSpec) == 0 {
		subnets, err = s.buildSubnetsForZones()
		if err != nil {
			return false, fmt.Errorf("error failed building control plane subnets: %w", err)
		}
	} else {
		subnets = s.IBMVPCCluster.Spec.NetworkSpec.ControlPlaneSubnetsSpec
	}

	// Reconcile Control Plane subnets
	requeue := false
	for _, subnet := range subnets {
		requiresReconcile, err := s.reconcileSubnet(subnet, true)
		if err != nil {
			return false, fmt.Errorf("error failed reconciling control plane subnet: %w", err)
		}
		// If the reconcile of the subnet requires further reconciliation, plan to requeue entire ReconcileSubnets call, but attempt to further reconcile additional Subnets (prevent single subnet reconciliation in each call)
		if requiresReconcile {
			requeue = true
		}
	}

	// If no Compute subnets were supplied, attempt to create one in each zone.
	if len(s.IBMVPCCluster.Spec.NetworkSpec.ComputeSubnetsSpec) == 0 {
		// If neither Control Plane nor Compute subnets were supplied, we rely on both Planes using the same subnet per zone, and we will re-reconcile those subnets below, for IBMVPCCluster Status updates
		if len(s.IBMVPCCluster.Spec.NetworkSpec.ControlPlaneSubnetsSpec) != 0 {
			subnets, err = s.buildSubnetsForZones()
			if err != nil {
				return false, fmt.Errorf("error failed building compute subnets: %w", err)
			}
		}
	} else {
		subnets = s.IBMVPCCluster.Spec.NetworkSpec.ComputeSubnetsSpec
	}

	// Reconcile Compute subnets
	for _, subnet := range subnets {
		requiresReconcile, err := s.reconcileSubnet(subnet, false)
		if err != nil {
			return false, fmt.Errorf("error failed reconciling compute subnet: %w", err)
		}

		// If the reconcile of the subnet requires further reconciliation, plan to requeue entire ReconcileSubnets call, but attempt to further reconcile additional Subnets (prevent single reconciliation)
		if requiresReconcile {
			requeue = true
		}
	}

	// Return whether or not one or more subnets required further reconciling after attempting to process all Control Plane and Compute subnets.
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
// The logic can handle either Control Plane or Compute subnets, but must distinguish between them for Status updates.
func (s *VPCClusterScope) reconcileSubnet(subnet infrav1beta2.Subnet, isControlPlane bool) (bool, error) { //nolint: gocyclo
	var subnetID *string
	// If subnet already has an ID defined, use that for lookup
	if subnet.ID != nil {
		subnetID = subnet.ID
	} else {
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
				if status, ok := s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets[*subnet.Name]; ok && status.Ready {
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
	if subnetDetails.ID == nil {
		s.Info("error failed creating subnet", "name", subnet.Name)
		return false, fmt.Errorf("error failed creating subnet")
	} else if subnetDetails.CRN == nil {
		s.Info("error new subnet CRN missing", "name", subnet.Name)
		return false, fmt.Errorf("error failure during subnet creation")
	}
	s.Info("created subnet", "id", subnetID)

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
		if s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets == nil {
			s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets = make(map[string]*infrav1beta2.VPCResourceStatus)
		}
		s.IBMVPCCluster.Status.NetworkStatus.ComputeSubnets[*subnetDetails.ID] = subnetResourceStatus
	}

	// Add a tag to the subnet for the cluster
	err = s.TagResource(s.IBMVPCCluster.Name, *subnetDetails.CRN)
	if err != nil {
		return false, err
	}

	// Recommend we requeue reconciliation after subnet was successfully created
	return true, nil
}

/*
// ReconcileVPCSubnet reconciles VPC subnet.
func (s *VPCClusterScope) ReconcileVPCSubnet() (bool, error) {
	subnets := make([]infrav1beta2.Subnet, 0)
	// check whether user has set the vpc subnets
	if len(s.IBMVPCCluster.Spec.VPCSubnets) == 0 {
		// if the user did not set any subnet, we try to create subnet in all the zones.
		powerVSZone := s.Zone()
		if powerVSZone == nil {
			return false, fmt.Errorf("error reconicling vpc subnet, powervs zone is not set")
		}
		region := endpoints.ConstructRegionFromZone(*powerVSZone)
		vpcZones, err := genUtil.VPCZonesForPowerVSRegion(region)
		if err != nil {
			return false, err
		}
		if len(vpcZones) == 0 {
			return false, fmt.Errorf("error reconicling vpc subnet,error getting vpc zones, no zone found for region %s", region)
		}
		for _, zone := range vpcZones {
			subnet := infrav1beta2.Subnet{
				Name: ptr.To(fmt.Sprintf("%s-%s", *s.GetServiceName(infrav1beta2.ResourceTypeSubnet), zone)),
				Zone: ptr.To(zone),
			}
			subnets = append(subnets, subnet)
		}
	}
	for index, subnet := range s.IBMVPCCluster.Spec.VPCSubnets {
		if subnet.Name == nil {
			subnet.Name = ptr.To(fmt.Sprintf("%s-%d", *s.GetServiceName(infrav1beta2.ResourceTypeSubnet), index))
		}
		subnets = append(subnets, subnet)
	}
	for _, subnet := range subnets {
		s.Info("Reconciling vpc subnet", "subnet", subnet)
		var subnetID *string
		if subnet.ID != nil {
			subnetID = subnet.ID
		} else {
			subnetID = s.GetVPCSubnetID(*subnet.Name)
		}
		if subnetID != nil {
			subnetDetails, _, err := s.IBMVPCClient.GetSubnet(&vpcv1.GetSubnetOptions{
				ID: subnetID,
			})
			if err != nil {
				return false, err
			}
			if subnetDetails == nil {
				return false, fmt.Errorf("error failed to get vpc subnet with id %s", *subnetID)
			}
			// check for next subnet
			continue
		}

		// check VPC subnet exist in cloud
		vpcSubnetID, err := s.checkVPCSubnet(*subnet.Name)
		if err != nil {
			s.Error(err, "error checking VPC subnet in IBM Cloud")
			return false, err
		}
		if vpcSubnetID != "" {
			s.Info("Found VPC subnet in IBM Cloud", "id", vpcSubnetID)
			s.SetVPCSubnetID(*subnet.Name, infrav1beta2.ResourceReference{ID: &vpcSubnetID, ControllerCreated: ptr.To(false)})
			// check for next subnet
			continue
		}
		subnetID, err = s.createVPCSubnet(subnet)
		if err != nil {
			s.Error(err, "error creating vpc subnet")
			return false, err
		}
		s.Info("created vpc subnet", "id", subnetID)
		s.SetVPCSubnetID(*subnet.Name, infrav1beta2.ResourceReference{ID: subnetID, ControllerCreated: ptr.To(true)})
		return true, nil
	}
	return false, nil
}

// checkVPCSubnet checks VPC subnet exist in cloud.
func (s *VPCClusterScope) checkVPCSubnet(subnetName string) (string, error) {
	vpcSubnet, err := s.IBMVPCClient.GetVPCSubnetByName(subnetName)
	if err != nil {
		return "", err
	}
	if vpcSubnet == nil {
		return "", nil
	}
	return *vpcSubnet.ID, nil
}.
*/

// createSubnet creates a new VPC subnet.
func (s *VPCClusterScope) createSubnet(subnet infrav1beta2.Subnet) (*vpcv1.Subnet, error) {
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
	cidrBlock, err := s.VPCClient.GetSubnetAddrPrefix(*vpcID, *subnet.Zone)
	if err != nil {
		return nil, err
	}
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
		IPVersion:     ptr.To(ipVersion),
		Ipv4CIDRBlock: ptr.To(cidrBlock),
		Name:          subnet.Name,
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
		return nil, fmt.Errorf("create subnet returned nil")
	}
	return subnetDetails, nil
}

/*

// ReconcileLoadBalancer reconcile loadBalancer.
func (s *VPCClusterScope) ReconcileLoadBalancer() (bool, error) {
	loadBalancers := make([]infrav1beta2.VPCLoadBalancerSpec, 0)
	if len(s.IBMVPCCluster.Spec.LoadBalancers) == 0 {
		loadBalancer := infrav1beta2.VPCLoadBalancerSpec{
			Name:   *s.GetServiceName(infrav1beta2.ResourceTypeLoadBalancer),
			Public: ptr.To(true),
		}
		loadBalancers = append(loadBalancers, loadBalancer)
	}
	for index, loadBalancer := range s.IBMVPCCluster.Spec.LoadBalancers {
		if loadBalancer.Name == "" {
			loadBalancer.Name = fmt.Sprintf("%s-%d", *s.GetServiceName(infrav1beta2.ResourceTypeLoadBalancer), index)
		}
		loadBalancers = append(loadBalancers, loadBalancer)
	}

	for _, loadBalancer := range loadBalancers {
		var loadBalancerID *string
		if loadBalancer.ID != nil {
			loadBalancerID = loadBalancer.ID
		} else {
			loadBalancerID = s.GetLoadBalancerID(loadBalancer.Name)
		}
		if loadBalancerID != nil {
			s.Info("LoadBalancer ID is set, fetching loadbalancer details", "loadbalancerid", *loadBalancerID)
			loadBalancer, _, err := s.IBMVPCClient.GetLoadBalancer(&vpcv1.GetLoadBalancerOptions{
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
			s.SetLoadBalancerStatus(*loadBalancer.Name, loadBalancerStatus)
			continue
		}
		// check VPC load balancer exist in cloud
		loadBalancerStatus, err := s.checkLoadBalancer(loadBalancer)
		if err != nil {
			return false, err
		}
		if loadBalancerStatus != nil {
			s.SetLoadBalancerStatus(loadBalancer.Name, *loadBalancerStatus)
			continue
		}
		// create loadBalancer
		loadBalancerStatus, err = s.createLoadBalancer(loadBalancer)
		if err != nil {
			return false, err
		}
		s.Info("Created VPC load balancer", "id", loadBalancerStatus.ID)
		s.SetLoadBalancerStatus(loadBalancer.Name, *loadBalancerStatus)
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
	loadBalancer, err := s.IBMVPCClient.GetLoadBalancerByName(lb.Name)
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
	resourceGroupID := s.GetResourceGroupID()
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

	subnetIDs := s.GetVPCSubnetIDs()
	if subnetIDs == nil {
		return nil, fmt.Errorf("error subnet required for load balancer creation")
	}
	for _, subnetID := range subnetIDs {
		subnet := &vpcv1.SubnetIdentity{
			ID: subnetID,
		}
		options.Subnets = append(options.Subnets, subnet)
	}
	options.SetPools([]vpcv1.LoadBalancerPoolPrototype{
		{
			Algorithm:     core.StringPtr("round_robin"),
			HealthMonitor: &vpcv1.LoadBalancerPoolHealthMonitorPrototype{Delay: core.Int64Ptr(5), MaxRetries: core.Int64Ptr(2), Timeout: core.Int64Ptr(2), Type: core.StringPtr("tcp")},
			// Note: Appending port number to the name, it will be referenced to set target port while adding new pool member
			Name:     core.StringPtr(fmt.Sprintf("%s-pool-%d", lb.Name, s.APIServerPort())),
			Protocol: core.StringPtr("tcp"),
		},
	})

	options.SetListeners([]vpcv1.LoadBalancerListenerPrototypeLoadBalancerContext{
		{
			Protocol: core.StringPtr("tcp"),
			Port:     core.Int64Ptr(int64(s.APIServerPort())),
			DefaultPool: &vpcv1.LoadBalancerPoolIdentityByName{
				Name: core.StringPtr(fmt.Sprintf("%s-pool-%d", lb.Name, s.APIServerPort())),
			},
		},
	})

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

	loadBalancer, _, err := s.IBMVPCClient.CreateLoadBalancer(options)
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

// COSInstance returns the COS instance reference.
func (s *VPCClusterScope) COSInstance() *infrav1beta2.CosInstance {
	return s.IBMVPCCluster.Spec.CosInstance
}

// ReconcileCOSInstance reconcile COS bucket.
func (s *VPCClusterScope) ReconcileCOSInstance() error {
	// check COS service instance exist in cloud
	cosServiceInstanceStatus, err := s.checkCOSServiceInstance()
	if err != nil {
		return err
	}
	if cosServiceInstanceStatus != nil {
		s.SetStatus(infrav1beta2.ResourceTypeCOSInstance, infrav1beta2.ResourceReference{ID: cosServiceInstanceStatus.GUID, ControllerCreated: ptr.To(false)})
	} else {
		// create COS service instance
		cosServiceInstanceStatus, err = s.createCOSServiceInstance()
		if err != nil {
			s.Error(err, "error creating cos service instance")
			return err
		}
		s.Info("Created COS service instance", "id", cosServiceInstanceStatus.GUID)
		s.SetStatus(infrav1beta2.ResourceTypeCOSInstance, infrav1beta2.ResourceReference{ID: cosServiceInstanceStatus.GUID, ControllerCreated: ptr.To(true)})
	}

	props, err := authenticator.GetProperties()
	if err != nil {
		s.Error(err, "error while fetching service properties")
		return err
	}

	apiKey, ok := props["APIKEY"]
	if !ok {
		return fmt.Errorf("ibmcloud api key is not provided, set %s environmental variable", "IBMCLOUD_API_KEY")
	}

	region := s.bucketRegion()
	if region == "" {
		return fmt.Errorf("failed to determine cos bucket region, both buckeet region and vpc region not set")
	}

	serviceEndpoint := fmt.Sprintf("s3.%s.%s", region, cosURLDomain)
	// Fetch the COS service endpoint.
	cosServiceEndpoint := endpoints.FetchEndpoints(string(endpoints.COS), s.ServiceEndpoint)
	if cosServiceEndpoint != "" {
		s.Logger.V(3).Info("Overriding the default COS endpoint", "cosEndpoint", cosServiceEndpoint)
		serviceEndpoint = cosServiceEndpoint
	}

	cosOptions := cos.ServiceOptions{
		Options: &cosSession.Options{
			Config: aws.Config{
				Endpoint: &serviceEndpoint,
				Region:   &region,
			},
		},
	}

	cosClient, err := cos.NewService(cosOptions, apiKey, *cosServiceInstanceStatus.GUID)
	if err != nil {
		return fmt.Errorf("failed to create cos client: %w", err)
	}
	s.COSClient = cosClient

	// check bucket exist in service instance
	if exist, err := s.checkCOSBucket(); exist {
		return nil
	} else if err != nil {
		s.Error(err, "error checking cos bucket")
		return err
	}

	// create bucket in service instance
	if err := s.createCOSBucket(); err != nil {
		s.Error(err, "error creating cos bucket")
		return err
	}
	return nil
}

func (s *VPCClusterScope) checkCOSBucket() (bool, error) {
	if _, err := s.COSClient.GetBucketByName(*s.GetServiceName(infrav1beta2.ResourceTypeCOSBucket)); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket, "Forbidden", "NotFound":
				// If the bucket doesn't exist that's ok, we'll try to create it
				return false, nil
			default:
				return false, err
			}
		} else {
			return false, err
		}
	}
	return true, nil
}

func (s *VPCClusterScope) checkCOSServiceInstance() (*resourcecontrollerv2.ResourceInstance, error) {
	// check cos service instance
	serviceInstance, err := s.ResourceClient.GetInstanceByName(*s.GetServiceName(infrav1beta2.ResourceTypeCOSInstance), resourcecontroller.CosResourceID, resourcecontroller.CosResourcePlanID)
	if err != nil {
		return nil, err
	}
	if serviceInstance == nil {
		s.Info("cos service instance is nil", "name", *s.GetServiceName(infrav1beta2.ResourceTypeCOSInstance))
		return nil, nil
	}
	if *serviceInstance.State != string(infrav1beta2.ServiceInstanceStateActive) {
		s.Info("cos service instance not in active state", "current state", *serviceInstance.State)
		return nil, fmt.Errorf("cos instance not in active state, current state: %s", *serviceInstance.State)
	}
	return serviceInstance, nil
}

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
}

// resourceCreatedByController helps to identify resource created by controller or not.
func (s *VPCClusterScope) isResourceCreatedByController(resourceType infrav1beta2.ResourceType) bool { //nolint:gocyclo
	switch resourceType {
	case infrav1beta2.ResourceTypeVPC:
		vpcStatus := s.IBMVPCCluster.Status.VPC
		if vpcStatus == nil || vpcStatus.ControllerCreated == nil || !*vpcStatus.ControllerCreated {
			return false
		}
		return true
	case infrav1beta2.ResourceTypeServiceInstance:
		serviceInstance := s.IBMVPCCluster.Status.ServiceInstance
		if serviceInstance == nil || serviceInstance.ControllerCreated == nil || !*serviceInstance.ControllerCreated {
			return false
		}
		return true
	}
	return false
}

// TODO: duplicate function, optimize it.
func (s *VPCClusterScope) bucketRegion() string {
	if s.COSInstance() != nil && s.COSInstance().BucketRegion != "" {
		return s.COSInstance().BucketRegion
	}
	// if the bucket region is not set, use vpc region
	vpcDetails := s.VPC()
	if vpcDetails != nil && vpcDetails.Region != nil {
		return *vpcDetails.Region
	}
	return ""
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

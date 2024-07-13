/*
Copyright 2022 The Kubernetes Authors.

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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	// ClusterFinalizer allows DockerClusterReconciler to clean up resources associated with DockerCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "ibmvpccluster.infrastructure.cluster.x-k8s.io"
)

// IBMVPCClusterSpec defines the desired state of IBMVPCCluster.
type IBMVPCClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The IBM Cloud Region the cluster lives in.
	Region string `json:"region"`

	// The VPC resources should be created under the resource group.
	ResourceGroup string `json:"resourceGroup"`

	// The Name of VPC.
	VPC string `json:"vpc,omitempty"`

	// The Name of availability zone.
	Zone string `json:"zone,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint capiv1beta1.APIEndpoint `json:"controlPlaneEndpoint"`

	// ControlPlaneLoadBalancer is optional configuration for customizing control plane behavior.
	// +optional
	ControlPlaneLoadBalancer *VPCLoadBalancerSpec `json:"controlPlaneLoadBalancer,omitempty"`

	// image represents the Image details used for the cluster.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// loadBalancers is a set of VPC Load Balancers definition to use for the cluster.
	// +optional
	LoadBalancers []*VPCLoadBalancerSpec `json:"loadbalancers,omitempty"`

	// network represents the VPC network to use for the cluster.
	// +optional
	Network *VPCNetworkSpec `json:"network,omitempty"`
}

// VPCLoadBalancerSpec defines the desired state of an VPC load balancer.
type VPCLoadBalancerSpec struct {
	// Name sets the name of the VPC load balancer.
	// +kubebuilder:validation:MinLength:=1
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:validation:Pattern=`^([a-z]|[a-z][-a-z0-9]*[a-z0-9])$`
	// +optional
	Name string `json:"name,omitempty"`

	// id of the loadbalancer
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength:=64
	// +kubebuilder:validation:Pattern=`^[-0-9a-z_]+$`
	// +optional
	ID *string `json:"id,omitempty"`

	// public indicates that load balancer is public or private
	// +kubebuilder:default=true
	// +optional
	Public *bool `json:"public,omitempty"`

	// AdditionalListeners sets the additional listeners for the control plane load balancer.
	// +listType=map
	// +listMapKey=port
	// +optional
	// ++kubebuilder:validation:UniqueItems=true
	AdditionalListeners []AdditionalListenerSpec `json:"additionalListeners,omitempty"`

	// backendPools defines the LB's backend pools.
	// +optional
	BackendPools []BackendPoolSpec `json:"backendPools,omitempty"`
}

// AdditionalListenerSpec defines the desired state of an
// additional listener on an VPC load balancer.
type AdditionalListenerSpec struct {
	// Port sets the port for the additional listener.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int64 `json:"port"`
}

// BackendPoolSpec defines the desired configuration of a VPC Load Balancer Backend Pool.
type BackendPoolSpec struct {
	// name defines the name of the Backend Pool.
	// +optional
	Name *string `json:"name,omitempty"`

	// algorithm defines the load balancing algorithm to use.
	// +required
	Algorithm string `json:"algorithm"`

	// protocol defines the protocol to use for the Backend Pool.
	// +required
	Protocol string `json:"protocol"`

	// healthDelay defines the seconds to wait between health checks.
	// +required
	HealthDelay int64 `json:"healthDelay"`

	// healthRetries defines the max retries for health check.
	// +required
	HealthRetries int64 `json:"healthRetries"`

	// healthTimeout defines the seconds to wait for a health check response.
	// +required
	HealthTimeout int64 `json:"healthTimeout"`

	// healthType defines the protocol used for health checks.
	// +required
	HealthType string `json:"healthType"`

	// healthMonitorURL defines the URL to use for health monitoring.
	// +optional
	HealthMonitorURL *string `json:"healthMonitorURL,omitempty"`

	// healthMonitorPort defines the port to perform health monitoring on.
	// +optional
	HealthMonitorPort *int64 `json:"healthMonitorPort,omitempty"`
}

// VPCSecurityGroupStatus defines a vpc security group resource status with its id and respective rule's ids.
type VPCSecurityGroupStatus struct {
	// id represents the id of the resource.
	ID *string `json:"id,omitempty"`
	// rules contains the id of rules created under the security group
	RuleIDs []*string `json:"ruleIDs,omitempty"`
	// +kubebuilder:default=false
	// controllerCreated indicates whether the resource is created by the controller.
	ControllerCreated *bool `json:"controllerCreated,omitempty"`
}

// VPCLoadBalancerStatus defines the status VPC load balancer.
type VPCLoadBalancerStatus struct {
	// id of VPC load balancer.
	// +optional
	ID *string `json:"id,omitempty"`
	// State is the status of the load balancer.
	State VPCLoadBalancerState `json:"state,omitempty"`
	// hostname is the hostname of load balancer.
	// +optional
	Hostname *string `json:"hostname,omitempty"`
	// +kubebuilder:default=false
	// controllerCreated indicates whether the resource is created by the controller.
	ControllerCreated *bool `json:"controllerCreated,omitempty"`
}

// ImageSpec defines the desired state of the VPC Custom Image resources for the cluster.
// +kubebuilder:validation:XValidation:rule="(has(self.cosInstance) || (has(self.cosBucket) || (has(self.cosObject)) && (!has(self.cosInstance) || !has(self.cosBucket) || !has(self.cosObject))",message="if any of cosInstance, cosBucket, or cosObject are specified, all must be specified"
type ImageSpec struct {
	// name is the name of the desired VPC Custom Image.
	// +required
	Name string `json:"name"`

	// cosInstance is the name of the IBM Cloud COS Instance containing the source of the image, if necessary.
	// +optional
	COSInstance *string `json:"cosInstance,omitempty"`

	// cosBucket is the name of the IBM Cloud COS Bucket containing the source of the image, if necessary.
	// +optional
	COSBucket *string `json:"cosBucket,omitempty"`

	// cosBucketRegion is the COS region the bucket is in.
	// +optional
	COSBucketRegion *string `json:"cosBucketRegion,omitempty"`

	// cosObject is the name of a IBM Cloud COS Object used as the source of the image, if necessary.
	// +optional
	COSObject *string `json:"cosObject,omitempty"`

	// operatingSystem is the Custom Image's Operating System name.
	// +optional
	OperatingSystem *string `json:"operatingSystem,omitempty"`

	// resourceGroup is the Resource Group to create the Custom Image in.
	// +optional
	ResourceGroup *GenericResourceReference `json:"resourceGroup,omitempty"`
}

// VPCNetworkSpec defines the desired state of the network resources for the cluster.
type VPCNetworkSpec struct {
	// workerSubnets is a set of Subnet's which define the Compute subnets.
	// +optional
	WorkerSubnets []Subnet `json:"workerSubnets,omitempty"`

	// controlPlaneSubnets is a set of Subnet's which define the Control Plane subnets.
	// +optional
	ControlPlaneSubnets []Subnet `json:"controlPlaneSubnets,omitempty"`

	// loadBalancers is a set of VPC Load Balancers definition to use for the cluster.
	// +optional
	LoadBalancers []VPCLoadBalancerSpec `json:"loadbalancers,omitempty"`

	// resourceGroup is the name of the Resource Group containing all of the newtork resources.
	// This can be different than the Resource Group containing the remaining cluster resources.
	// +optional
	ResourceGroup *string `json:"resourceGroup,omitempty"`

	// securityGroups is a set of VPCSecurityGroup's which define the VPC Security Groups that manage traffic within and out of the VPC.
	// +optional
	SecurityGroups []VPCSecurityGroup `json:"securityGroups,omitempty"`

	// vpc defines the IBM Cloud VPC.
	// +optional
	VPC *VPCResource `json:"vpc,omitempty"`
}

// IBMVPCClusterStatus defines the observed state of IBMVPCCluster.
type IBMVPCClusterStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions defines current service state of the load balancer.
	// +optional
	Conditions capiv1beta1.Conditions `json:"conditions,omitempty"`

	// ControlPlaneLoadBalancerState is the status of the load balancer.
	// dep: rely on NetworkStatus instead.
	// +optional
	ControlPlaneLoadBalancerState VPCLoadBalancerState `json:"controlPlaneLoadBalancerState,omitempty"`

	// imageStatus is the status of the VPC Custom Image.
	// +optional
	ImageStatus *VPCResourceStatus `json:"imageStatus,omitempty"`

	// networkStatus is the status of the VPC network in its entirety resources.
	NetworkStatus *VPCNetworkStatus `json:"networkStatus,omitempty"`

	// ready is true when the provider resource is ready.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// resourceGroup is the reference to the IBM Cloud VPC resource group under which the resources will be created.
	ResourceGroup *GenericResourceReference `json:"resourceGroupID,omitempty"`

	// dep: rely on NetworkStatus instead.
	Subnet Subnet `json:"subnet,omitempty"`

	// dep: rely on NetworkStatus instead.
	VPC VPC `json:"vpc,omitempty"`

	// dep: rely on NetworkStatus instead.
	VPCEndpoint VPCEndpoint `json:"vpcEndpoint,omitempty"`
}

// VPCNetworkStatus provides details on the status of VPC network resources.
type VPCNetworkStatus struct {
	// workerSubnets references the VPC Subnets for the cluster's workers.
	// The map simplifies lookups, using the VPCResourceStatus.Name as the key.
	// +optional
	WorkerSubnets map[string]*VPCResourceStatus `json:"workerSubnets,omitempty"`

	// controlPlaneSubnets references the VPC Subnets for the cluster's Control Plane.
	// The map is simplifies lookups, using the VPCResourceStatus.Name as the key.
	// +optional
	ControlPlaneSubnets map[string]*VPCResourceStatus `json:"controlPlaneSubnets,omitempty"`

	// loadBalancers references the VPC Load Balancer's for the cluster.
	// The map simplifies lookups.
	// +optional
	LoadBalancers map[string]*VPCLoadBalancerStatus `json:"loadBalancers,omitempty"`

	// resourceGroup references the Resource Group for Network resources for the cluster.
	// This can be the same or unique from the cluster's Resource Group.
	// +optional
	ResourceGroup *GenericResourceReference `json:"resourceGroup,omitempty"`

	// securityGroups references the VPC Security Groups for the cluster.
	// The map simplifies lookups.
	// +optional
	SecurityGroups map[string]*VPCResourceStatus `json:"securityGroups,omitempty"`

	// vpc references the IBM Cloud VPC.
	// +optional
	VPC *VPCResourceStatus `json:"vpc,omitempty"`
}

// VPCResourceStatus identifies a resource by crn and type and whether it was created by the controller.
type VPCResourceStatus struct {
	// id defines the IBM Cloud ID of the resource.
	// +required
	ID string `json:"id"`

	// name defines the name of the resource.
	// +optional
	Name string `json:"name,omitempty"`

	// ready defines whether the IBM Cloud VPC resource is ready.
	// +required
	Ready bool `json:"ready"`
}

// VPC holds the VPC information.
type VPC struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ibmvpcclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this IBMVPCCluster belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready for IBM VPC instances"

// IBMVPCCluster is the Schema for the ibmvpcclusters API.
type IBMVPCCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IBMVPCClusterSpec   `json:"spec,omitempty"`
	Status IBMVPCClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IBMVPCClusterList contains a list of IBMVPCCluster.
type IBMVPCClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IBMVPCCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IBMVPCCluster{}, &IBMVPCClusterList{})
}

// GetConditions returns the observations of the operational state of the IBMVPCCluster resource.
func (r *IBMVPCCluster) GetConditions() capiv1beta1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the underlying service state of the IBMVPCCluster to the predescribed clusterv1.Conditions.
func (r *IBMVPCCluster) SetConditions(conditions capiv1beta1.Conditions) {
	r.Status.Conditions = conditions
}

// Set will update a GenericResourceReference values with those provided.
func (r *GenericResourceReference) Set(resource GenericResourceReference) {
	r.ID = resource.ID
}

// Set will update a VPCResourceStatus values with those provided.
func (s *VPCResourceStatus) Set(vpcResource VPCResourceStatus) {
	s.ID = vpcResource.ID
	s.Name = vpcResource.Name
	s.Ready = vpcResource.Ready
}

package ibmcloud

import (
	"fmt"

	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"

	"github.com/openshift/installer/pkg/types"
)

const (
	// KubernetesAPIPort is the Kubernetes API port.
	KubernetesAPIPort = 6443

	// KubernetesAPIPrivatePostfix is the name postfix for Kubernetes API Private LB resources.
	KubernetesAPIPrivatePostfix = "kubernetes-api-private"

	// KubernetesAPIPublicPostfix is the name postfix for Kubernetes API Public LB resources.
	KubernetesAPIPublicPostfix = "kubernetes-api-public"

	// MachineConfigPostfix is the name postfix for Machine Config Server LB resources.
	MachineConfigPostfix = "machine-config"

	// MachineConfigServerPort is the Machine Config Server port.
	MachineConfigServerPort = 22623

	healthMonitorURLReadyz = "/readyz"
)

func getLoadBalancers(infraID string, securityGroups []capibmcloud.VPCResource, subnets []capibmcloud.VPCResource, publish types.PublishingStrategy) []capibmcloud.VPCLoadBalancerSpec {
	loadBalancers := make([]capibmcloud.VPCLoadBalancerSpec, 0, 2)

	loadBalancers = append(loadBalancers, buildPrivateLoadBalancer(infraID, securityGroups, subnets))
	if publish == types.ExternalPublishingStrategy {
		loadBalancers = append(loadBalancers, buildPublicLoadBalancer(infraID, securityGroups, subnets))
	}

	return loadBalancers
}

func buildPrivateLoadBalancer(infraID string, securityGroups []capibmcloud.VPCResource, subnets []capibmcloud.VPCResource) capibmcloud.VPCLoadBalancerSpec {
	kubeAPIBackendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, KubernetesAPIPrivatePostfix))
	machineConfigBackendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, MachineConfigPostfix))

	return capibmcloud.VPCLoadBalancerSpec{
		Name:   fmt.Sprintf("%s-%s", infraID, KubernetesAPIPrivatePostfix),
		Public: ptr.To(false),
		AdditionalListeners: []capibmcloud.AdditionalListenerSpec{
			{
				DefaultPoolName: kubeAPIBackendPoolNamePtr,
				Port:            KubernetesAPIPort,
				Protocol:        &capibmcloud.VPCLoadBalancerListenerProtocolTCP,
			},
			{
				DefaultPoolName: machineConfigBackendPoolNamePtr,
				Port:            MachineConfigServerPort,
				Protocol:        &capibmcloud.VPCLoadBalancerListenerProtocolTCP,
			},
		},
		BackendPools: []capibmcloud.VPCLoadBalancerBackendPoolSpec{
			{
				// Kubernetes API pool
				Name:      kubeAPIBackendPoolNamePtr,
				Algorithm: capibmcloud.VPCLoadBalancerBackendPoolAlgorithmRoundRobin,
				Protocol:  capibmcloud.VPCLoadBalancerBackendPoolProtocolTCP,
				HealthMonitor: capibmcloud.VPCLoadBalancerHealthMonitorSpec{
					Delay:   60,
					Retries: 5,
					Timeout: 30,
					Type:    capibmcloud.VPCLoadBalancerBackendPoolHealthMonitorTypeHTTPS,
					URLPath: ptr.To(healthMonitorURLReadyz),
				},
			},
			{
				// Machine Config Server pool
				Name:      machineConfigBackendPoolNamePtr,
				Algorithm: capibmcloud.VPCLoadBalancerBackendPoolAlgorithmRoundRobin,
				Protocol:  capibmcloud.VPCLoadBalancerBackendPoolProtocolTCP,
				HealthMonitor: capibmcloud.VPCLoadBalancerHealthMonitorSpec{
					Delay:   60,
					Retries: 5,
					Timeout: 30,
					Type:    capibmcloud.VPCLoadBalancerBackendPoolHealthMonitorTypeHTTPS,
					URLPath: ptr.To(healthMonitorURLReadyz),
				},
			},
		},
		SecurityGroups: securityGroups,
		Subnets:        subnets,
	}
}

func buildPublicLoadBalancer(infraID string, securityGroups []capibmcloud.VPCResource, subnets []capibmcloud.VPCResource) capibmcloud.VPCLoadBalancerSpec {
	backendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, KubernetesAPIPublicPostfix))

	return capibmcloud.VPCLoadBalancerSpec{
		Name:   fmt.Sprintf("%s-%s", infraID, KubernetesAPIPublicPostfix),
		Public: ptr.To(true),
		AdditionalListeners: []capibmcloud.AdditionalListenerSpec{
			{
				DefaultPoolName: backendPoolNamePtr,
				Port:            KubernetesAPIPort,
				Protocol:        &capibmcloud.VPCLoadBalancerListenerProtocolTCP,
			},
		},
		BackendPools: []capibmcloud.VPCLoadBalancerBackendPoolSpec{
			{
				// Kubernetes API pool
				Name:      backendPoolNamePtr,
				Algorithm: capibmcloud.VPCLoadBalancerBackendPoolAlgorithmRoundRobin,
				Protocol:  capibmcloud.VPCLoadBalancerBackendPoolProtocolTCP,
				HealthMonitor: capibmcloud.VPCLoadBalancerHealthMonitorSpec{
					Delay:   60,
					Retries: 5,
					Timeout: 30,
					Type:    capibmcloud.VPCLoadBalancerBackendPoolHealthMonitorTypeHTTPS,
					URLPath: ptr.To(healthMonitorURLReadyz),
				},
			},
		},
		SecurityGroups: securityGroups,
		Subnets:        subnets,
	}
}

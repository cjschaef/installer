package ibmcloud

import (
	"fmt"

	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"

	"github.com/openshift/installer/pkg/types"
)

const (
	// kubernetesAPIPort is the Kubernetes API port.
	kubernetesAPIPort = 6443

	// kubernetesAPIPrivatePostfix is the name postfix for Kubernetes API Private LB resources.
	kubernetesAPIPrivatePostfix = "kubernetes-api-private"

	// kubernetesAPIPublicPostfix is the name postfix for Kubernetes API Public LB resources.
	kubernetesAPIPublicPostfix = "kubernetes-api-public"

	// machineConfigPostfix is the name postfix for Machine Config Server LB resources.
	machineConfigPostfix = "machine-config"

	// machineConfigServerPort is the Machine Config Server port.
	machineConfigServerPort = 22623

	// algorithmRoundRobin is the Round-Robin distribution algorithm for LB Backend Pools.
	algorithmRoundRobin = "round_robin"

	// protocolTCP is the TCP protocol type for LB Backend Pools.
	protocolTCP = "tcp"

	healthTypeHTTPS = "https"

	healthMonitorURLReadyz = "/readyz"
)

func getLoadBalancers(infraID string, publish types.PublishingStrategy) []*capibmcloud.VPCLoadBalancerSpec {
	loadBalancers := make([]*capibmcloud.VPCLoadBalancerSpec, 0, 2)
	loadBalancers = append(loadBalancers, buildPrivateLoadBalancer(infraID))
	if publish == types.ExternalPublishingStrategy {
		loadBalancers = append(loadBalancers, buildPublicLoadBalancer(infraID))
	}

	return loadBalancers
}

func buildPrivateLoadBalancer(infraID string) *capibmcloud.VPCLoadBalancerSpec {
	return &capibmcloud.VPCLoadBalancerSpec{
		Name:   fmt.Sprintf("%s-%s", infraID, kubernetesAPIPrivatePostfix),
		Public: ptr.To(false),
		AdditionalListeners: []capibmcloud.AdditionalListenerSpec{
			{
				Port: kubernetesAPIPort,
			},
			{
				Port: machineConfigServerPort,
			},
		},
		BackendPools: []*capibmcloud.BackendPoolSpec{
			{
				// Kubernetes API pool
				Name:             ptr.To(fmt.Sprintf("%s-%s", infraID, kubernetesAPIPrivatePostfix)),
				Algorithm:        algorithmRoundRobin,
				Protocol:         protocolTCP,
				HealthDelay:      60,
				HealthRetries:    5,
				HealthTimeout:    30,
				HealthType:       healthTypeHTTPS,
				HealthMonitorURL: ptr.To(healthMonitorURLReadyz),
			},
			{
				// Machine Config Server pool
				Name:             ptr.To(fmt.Sprintf("%s-%s", infraID, machineConfigPostfix)),
				Algorithm:        algorithmRoundRobin,
				Protocol:         protocolTCP,
				HealthDelay:      60,
				HealthRetries:    5,
				HealthTimeout:    30,
				HealthType:       healthTypeHTTPS,
				HealthMonitorURL: ptr.To(healthMonitorURLReadyz),
			},
		},
	}
}

func buildPublicLoadBalancer(infraID string) *capibmcloud.VPCLoadBalancerSpec {
	return &capibmcloud.VPCLoadBalancerSpec{
		Name:   fmt.Sprintf("%s-%s", infraID, kubernetesAPIPublicPostfix),
		Public: ptr.To(true),
		AdditionalListeners: []capibmcloud.AdditionalListenerSpec{
			{
				Port: kubernetesAPIPort,
			},
		},
		BackendPools: []*capibmcloud.BackendPoolSpec{
			{
				// Kubernetes API pool
				Name:             ptr.To(fmt.Sprintf("%s-%s", infraID, kubernetesAPIPublicPostfix)),
				Algorithm:        algorithmRoundRobin,
				Protocol:         protocolTCP,
				HealthDelay:      60,
				HealthRetries:    5,
				HealthTimeout:    30,
				HealthType:       healthTypeHTTPS,
				HealthMonitorURL: ptr.To(healthMonitorURLReadyz),
			},
		},
	}
}

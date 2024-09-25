package ibmcloud

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
	ibmcloudmanifests "github.com/openshift/installer/pkg/asset/manifests/ibmcloud"
	"github.com/openshift/installer/pkg/types"
	ibmcloudprovider "github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1"
)

func GenerateMachines(ctx context.Context, infraID string, config *types.InstallConfig, subnets map[string]string, pool *types.MachinePool, imageName string, role string) ([]*asset.RuntimeFile, error) {
	machines, err := Machines(infraID, config, subnets, pool, role, fmt.Sprintf("%s-user-data", role))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s machines %w", role, err)
	}

	capibmcloudMachines := make([]*capibmcloud.IBMVPCMachine, 0, len(machines))
	result := make([]*asset.RuntimeFile, 0, len(machines))

	for _, machine := range machines {
		// For now, attempt to re-use MAPI machine spec
		providerSpec, ok := machine.Spec.ProviderSpec.Value.Object.(*ibmcloudprovider.IBMCloudMachineProviderSpec)
		if !ok {
			return nil, fmt.Errorf("unable to convert ProviderSpec to IBMCloudMachineProviderSpec")
		}

		// Generate the necessary machine data
		bootVolume := &capibmcloud.VPCVolume{}
		if providerSpec.BootVolume.EncryptionKey != "" {
			bootVolume.EncryptionKeyCRN = providerSpec.BootVolume.EncryptionKey
		}

		// Potentially move this to after capibmcloudMachine defining.
		var dedicatedHost *capibmcloud.VPCResource
		if providerSpec.DedicatedHost != "" {
			dedicatedHost = &capibmcloud.VPCResource{
				Name: ptr.To(providerSpec.DedicatedHost),
			}
		}
		image := &capibmcloud.IBMVPCResourceReference{
			Name: ptr.To(imageName),
		}

		// If these are Control Plane nodes, make sure they are included in the various LB backend pool members.
		var loadBalancerPoolMembers []capibmcloud.VPCLoadBalancerBackendPoolMember
		if role == "master" {
			kubeAPIBackendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, ibmcloudmanifests.KubernetesAPIPrivatePostfix))
			machineConfigBackendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, ibmcloudmanifests.MachineConfigPostfix))

			// Populate the Machine's LB Pool details.
			loadBalancerPoolMembers = []capibmcloud.VPCLoadBalancerBackendPoolMember{
				// Kubernetes API private pool.
				{
					LoadBalancer: capibmcloud.VPCResource{
						// LB and Pool have the same name format.
						Name: kubeAPIBackendPoolNamePtr,
					},
					Pool: capibmcloud.VPCResource{
						Name: kubeAPIBackendPoolNamePtr,
					},
					Port: ibmcloudmanifests.KubernetesAPIPort,
				},
				// Machine Config Server pool.
				{
					LoadBalancer: capibmcloud.VPCResource{
						Name: kubeAPIBackendPoolNamePtr,
					},
					Pool: capibmcloud.VPCResource{
						Name: machineConfigBackendPoolNamePtr,
					},
					Port: ibmcloudmanifests.MachineConfigServerPort,
				},
			}

			// If using External/Public cluster, add the Kubernetes API public pool as well.
			if config.Publish == types.ExternalPublishingStrategy {
				kubeAPIPublicBackendPoolNamePtr := ptr.To(fmt.Sprintf("%s-%s", infraID, ibmcloudmanifests.KubernetesAPIPublicPostfix))
				loadBalancerPoolMembers = append(loadBalancerPoolMembers, capibmcloud.VPCLoadBalancerBackendPoolMember{
					LoadBalancer: capibmcloud.VPCResource{
						Name: kubeAPIPublicBackendPoolNamePtr,
					},
					Pool: capibmcloud.VPCResource{
						Name: kubeAPIPublicBackendPoolNamePtr,
					},
					Port: ibmcloudmanifests.KubernetesAPIPort,
				})
			}
		}

		// Compile the list of security groups for machine.
		var securityGroups []capibmcloud.VPCResource
		if providerSpec.PrimaryNetworkInterface.SecurityGroups != nil && len(providerSpec.PrimaryNetworkInterface.SecurityGroups) > 0 {
			securityGroups = make([]capibmcloud.VPCResource, 0, len(providerSpec.PrimaryNetworkInterface.SecurityGroups))
			for _, securityGroupName := range providerSpec.PrimaryNetworkInterface.SecurityGroups {
				securityGroups = append(securityGroups, capibmcloud.VPCResource{
					Name: ptr.To(securityGroupName),
				})
			}
		}
		networkInterface := capibmcloud.NetworkInterface{
			SecurityGroups: securityGroups,
			Subnet:         providerSpec.PrimaryNetworkInterface.Subnet,
		}

		// TODO(cjschaef): Test SSH Key lookup
		/* var sshkeys []*capibmcloud.IBMVPCResourceReference
		sshkey, err := FindSSHKey(config.SSHKey, config.IBMCloud.Region, config.IBMCloud.ServiceEndpoints)
		if err != nil {
			return nil, fmt.Errorf("failure attempting to find sshkey for %s machines: %w", role, err)
		} else if sshkey != nil {
			sshkeys = []*capibmcloud.IBMVPCResourceReference{
				{
					ID: sshkey.ID,
				},
			}
		}
		.*/

		capibmcloudMachine := &capibmcloud.IBMVPCMachine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "IBMVPCMachine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: capiutils.Namespace,
				Name:      machine.Name,
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
				},
			},
			Spec: capibmcloud.IBMVPCMachineSpec{
				BootVolume:              bootVolume,
				DedicatedHost:           dedicatedHost,
				Image:                   image,
				LoadBalancerPoolMembers: loadBalancerPoolMembers,
				Name:                    machine.Name,
				PrimaryNetworkInterface: networkInterface,
				Profile:                 providerSpec.Profile,
				// SSHKeys:                 sshkeys,
				Zone: providerSpec.Zone,
			},
		}
		capibmcloudMachine.SetGroupVersionKind(capibmcloud.GroupVersion.WithKind("IBMVPCMachine"))
		capibmcloudMachines = append(capibmcloudMachines, capibmcloudMachine)

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_inframachine_%s.yaml", capibmcloudMachine.Name)},
			Object: capibmcloudMachine,
		})

		capiMachine := &capi.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: capiutils.Namespace,
				Name:      capibmcloudMachine.Name,
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
				},
			},
			Spec: capi.MachineSpec{
				ClusterName: infraID,
				Bootstrap: capi.Bootstrap{
					DataSecretName: ptr.To(fmt.Sprintf("%s-%s", infraID, role)),
				},
				InfrastructureRef: v1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "IBMVPCMachine",
					Name:       capibmcloudMachine.Name,
				},
			},
		}
		capiMachine.SetGroupVersionKind(capi.GroupVersion.WithKind("Machine"))

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_machine_%s.yaml", capiMachine.Name)},
			Object: capiMachine,
		})
	}

	// If we are generating Control Plane machines, we must also create a bootstrap machine as well
	if role == "master" {
		// Simply use the first Control Plane machine for bootstrap spec
		bootstrapSpec := capibmcloudMachines[0].Spec
		// Add bootstrap Security Group to PrimaryNetworkInterface.
		bootstrapSpec.PrimaryNetworkInterface.SecurityGroups = append(bootstrapSpec.PrimaryNetworkInterface.SecurityGroups, capibmcloud.VPCResource{
			Name: ptr.To(fmt.Sprintf("%s-%s", infraID, ibmcloudmanifests.BootstrapSGNamePostfix)),
		})

		bootstrapMachine := &capibmcloud.IBMVPCMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: capiutils.GenerateBoostrapMachineName(infraID),
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
				},
			},
			Spec: bootstrapSpec,
		}
		bootstrapMachine.SetGroupVersionKind(capibmcloud.GroupVersion.WithKind("IBMVPCMachine"))

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_inframachine_%s.yaml", bootstrapMachine.Name)},
			Object: bootstrapMachine,
		})

		bootstrapCAPIMachine := &capi.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: bootstrapMachine.Name,
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
				},
			},
			Spec: capi.MachineSpec{
				ClusterName: infraID,
				Bootstrap: capi.Bootstrap{
					DataSecretName: ptr.To(fmt.Sprintf("%s-bootstrap", infraID)),
				},
				InfrastructureRef: v1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "IBMVPCMachine",
					Name:       bootstrapMachine.Name,
				},
			},
		}
		bootstrapCAPIMachine.SetGroupVersionKind(capi.GroupVersion.WithKind("Machine"))

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_machine_%s.yaml", bootstrapMachine.Name)},
			Object: bootstrapCAPIMachine,
		})
	}

	return result, nil
}

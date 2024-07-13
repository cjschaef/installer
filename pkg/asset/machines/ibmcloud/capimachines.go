package ibmcloud

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capibmcloudv1 "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
	"github.com/openshift/installer/pkg/types"
	ibmcloudprovider "github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1"
)

func GenerateMachines(ctx context.Context, infraID string, config *types.InstallConfig, subnets map[string]string, pool *types.MachinePool, imageName string, role string) ([]*asset.RuntimeFile, error) {
	machines, err := Machines(infraID, config, subnets, pool, role, fmt.Sprintf("%s-user-data", role))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s machines %w", role, err)
	}

	capibmcloudMachines := make([]*capibmcloudv1.IBMVPCMachine, 0, len(machines))
	result := make([]*asset.RuntimeFile, 0, len(machines))

	for _, machine := range machines {
		// For now, attempt to re-use MAPI machine spec
		providerSpec, ok := machine.Spec.ProviderSpec.Value.Object.(*ibmcloudprovider.IBMCloudMachineProviderSpec)
		if !ok {
			return nil, fmt.Errorf("unable to convert ProviderSpec to IBMCloudMachineProviderSpec")
		}

		// Generate the necessary machine data
		bootVolume := &capibmcloudv1.VPCVolume{}
		if providerSpec.BootVolume.EncryptionKey != "" {
			bootVolume.EncryptionKeyCRN = providerSpec.BootVolume.EncryptionKey
		}
		//dedicatedHost := capibmcloudv1.IBMVPCResourceReference{
		//	Name: ptr.To(providerSpec.DedicatedHost),
		//}
		image := &capibmcloudv1.IBMVPCResourceReference{
			Name: ptr.To(imageName),
		}
		/*securityGroups := []*capibmcloudv1.IBMVPCResourceReference{}
		for _, sg := range providerSpec.PrimaryNetworkInterface.SecurityGroups{
			securityGroups = append(securityGroups, &capibmcloudv1.IBMVPCResourceReference{
				Name: ptr.To(sg),
			})
		}*/
		networkInterface := capibmcloudv1.NetworkInterface{
			// SecurityGroups: securityGroups,
			Subnet: providerSpec.PrimaryNetworkInterface.Subnet,
		}
		// TODO(cjschaef): See if we can lookup the IBM Cloud VPC SSH key (Name or ID) via the public key from InstallConfig
		// sshKeys := ...

		capibmcloudMachine := &capibmcloudv1.IBMVPCMachine{
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
			Spec: capibmcloudv1.IBMVPCMachineSpec{
				BootVolume: bootVolume,
				// DedicatedHost:           dedicatedHost,
				Image: image,
				Name:  machine.Name,
				// NetworkResourceGroup:    providerSpec.NetworkResourceGroup,
				PrimaryNetworkInterface: networkInterface,
				Profile:                 providerSpec.Profile,
				// Region:                  providerSpec.Region,
				// ResourceGroup:           providerSpec.ResourceGroup,
				// SSHKeys:                 sshKeys,
				// UserDataSecret:          fmt.Sprintf("%s-user-data", role),
				Zone: providerSpec.Zone,
			},
		}

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

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_machine_%s.yaml", capiMachine.Name)},
			Object: capiMachine,
		})
	}

	// If we are generating Control Plane machines, we must also create a bootstrap machine as well
	if role == "master" {
		// Simply use the first Control Plane machine for bootstrap spec
		bootstrapSpec := capibmcloudMachines[0].Spec
		bootstrapMachine := &capibmcloudv1.IBMVPCMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: capiutils.GenerateBoostrapMachineName(infraID),
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
				},
			},
			Spec: bootstrapSpec,
		}

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

		result = append(result, &asset.RuntimeFile{
			File:   asset.File{Filename: fmt.Sprintf("10_machine_%s.yaml", bootstrapMachine.Name)},
			Object: bootstrapCAPIMachine,
		})
	}

	return result, nil
}

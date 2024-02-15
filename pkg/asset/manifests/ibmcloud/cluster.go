package ibmcloud

import (
	"context"
	"fmt"
	"time"

	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"

	"github.com/openshift/installer/pkg/asset/installconfig"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
)

// GenerateClusterAssets generates the manifests for the cluster-api.
func GenerateClusterAssets(installConfig *installconfig.InstallConfig, clusterID *installconfig.ClusterID) (*capiutils.GenerateClusterAssetsOutput, error) {
	manifests := []*asset.RuntimeFile{}
	mainCIDR := capiutils.CIDRFromInstallConfig(installConfig)

	ibmcloudCluster := &capibmcloud.IBMVPCCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID.InfraID,
			Namespace: capiutils.Namespace,
		},
		Spec:
	}

	// If the install-config has subnets, use them instead.
	if len(installConfig.Platform.IBMCloud.Subnets) > 0 {
	}

	manifests = append(manifests, &asset.RuntimeFile{
		Object: ibmcloudCluster,
		File:   asset.File{Filename: "02_ibmcloud-cluster.yaml"},
	})

	return &capiutils.GenerateClusterAssetsOutput{
		Manifests: manifests,
		InfrastructureRef: &corev1.ObjectReference{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
			Kind:       "IBMCloudCluster",
			Name:       ibmcloudCluster.Name,
			Namespace:  ibmcloudCluster.Namespace,
		},
	}, nil
}

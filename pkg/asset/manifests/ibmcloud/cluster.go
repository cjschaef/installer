package ibmcloud

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/IBM/go-sdk-core/v5/core"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capibmcloud "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	ibmcloudic "github.com/openshift/installer/pkg/asset/installconfig/ibmcloud"
	"github.com/openshift/installer/pkg/asset/manifests/capiutils"
)

// GenerateClusterAssets generates the manifests for the cluster-api.
func GenerateClusterAssets(installConfig *installconfig.InstallConfig, clusterID *installconfig.ClusterID) (*capiutils.GenerateClusterAssetsOutput, error) {
	manifests := []*asset.RuntimeFile{}
	// mainCIDR := capiutils.CIDRFromInstallConfig(installConfig)
	platform := installConfig.Config.Platform.IBMCloud

	// Create IBM Cloud Credentials for IBM Cloud CAPI
	ibmcloudCreds := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ibmcloud-credentials",
			Namespace: capiutils.Namespace,
		},
		Data: make(map[string][]byte),
	}

	// Encode the API key prior to adding it to secret data
	encodedAPIKey := make([]byte, base64.StdEncoding.EncodedLen(len(os.Getenv("IC_API_KEY"))))
	base64.StdEncoding.Encode(encodedAPIKey, []byte(os.Getenv("IC_API_KEY")))

	credentialsData := fmt.Sprintf("IBMCLOUD_%s=%s\nIBMCLOUD_%s=%s", core.PROPNAME_AUTH_TYPE, core.AUTHTYPE_IAM, core.PROPNAME_APIKEY, encodedAPIKey)
	// If there is an endpoint override for IAM, we must inject it into the credentials data
	if len(platform.ServiceEndpoints) > 0 {
		for _, endpoint := range platform.ServiceEndpoints {
			if endpoint.Name == configv1.IBMCloudServiceIAM {
				credentialsData = fmt.Sprintf("%s\nIBMCLOUD_%s=%s", core.PROPNAME_AUTH_URL, credentialsData, endpoint.URL)
				break
			}
		}
	}
	ibmcloudCreds.Data[core.DEFAULT_CREDENTIAL_FILE_NAME] = []byte(credentialsData)

	manifests = append(manifests, &asset.RuntimeFile{
		Object: ibmcloudCreds,
		File:   asset.File{Filename: "01_ibmcloud-creds.yaml"},
	})

	resourceGroup := clusterID.InfraID
	if platform.ResourceGroupName != "" {
		resourceGroup = platform.ResourceGroupName
	}
	//networkResourceGroup := resourceGroup
	//if platform.NetworkResourceGroupName != "" {
	//	networkResourceGroup = platform.NetworkResourceGroupName
	//}

	

	ibmcloudCluster := &capibmcloud.IBMVPCCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterID.InfraID,
			Namespace: capiutils.Namespace,
		},
		Spec: capibmcloud.IBMVPCClusterSpec{
			ControlPlaneEndpoint: capi.APIEndpoint{
				Host: fmt.Sprintf("api.%s.%s", &installConfig.Config.ObjectMeta.Name, installConfig.Config.BaseDomain),
			},
			/*NetworkSpec: capibmcloud.IBMVPCNetworkSpec{
				ResourceGroup: networkResourceGroup,
				SecurityGroups: []capibmcloud.SecurityGroups{

				},
				ComputeSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: getSubnets(installConfig.IBMCloud.ComputeSubnets(context.TODO())),
				},
				ControlPlaneSubnetsSpec: &capibmcloud.IBMVPCSubnetsSpec{
					Subnets: getSubnets(installConfig.IBMCloud.ControlPlaneSubnets(context.TODO())),
				},

				VPC: &capibmcloud.IBMVPCResourceReference{
					Name: platform.GetVPCName(),
				},
			},*/
			Region:        platform.Region,
			ResourceGroup: resourceGroup,
		},
	}

	manifests = append(manifests, &asset.RuntimeFile{
		Object: ibmcloudCluster,
		File:   asset.File{Filename: "01_ibmcloud-cluster.yaml"},
	})

	return &capiutils.GenerateClusterAssetsOutput{
		Manifests: manifests,
		InfrastructureRef: &corev1.ObjectReference{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
			Kind:       "IBMVPCCluster",
			Name:       ibmcloudCluster.Name,
			Namespace:  ibmcloudCluster.Namespace,
		},
	}, nil
}

func getSubnets(subnets map[string]ibmcloudic.Subnet) []capibmcloud.Subnet {
	subnetList := make([]capibmcloud.Subnet, 0, len(subnets))
	for _, subnet := range subnets {
		subnetList = append(subnetList, capibmcloud.Subnet{
			Name: ptr.To(subnet.Name),
			Zone: ptr.To(subnet.Zone),
		})
	}
	return subnetList
}
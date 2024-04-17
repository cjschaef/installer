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

package resourcemanager

import (
	"fmt"
	"net/http"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"

	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/authenticator"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
)

// Service holds the IBM Cloud Resource Manager Service specific information.
type Service struct {
	client *resourcemanagerv2.ResourceManagerV2
}

// ServiceOptions holds the IBM Cloud Resource Manager Service Options specific information.
type ServiceOptions struct {
	*resourcemanagerv2.ResourceManagerV2Options
}

// ListResourceGroups will list all the Resource Groups.
func (s *Service) ListResourceGroups(options *resourcemanagerv2.ListResourceGroupsOptions) (*resourcemanagerv2.ResourceGroupList, *core.DetailedResponse, error) {
	return s.client.ListResourceGroups(options)
}

// GetResourceGroup will get the Resource Group.
func (s *Service) GetResourceGroup(options *resourcemanagerv2.GetResourceGroupOptions) (*resourcemanagerv2.ResourceGroup, *core.DetailedResponse, error) {
	return s.client.GetResourceGroup(options)
}

// CreateResourceGroup creates a new Resource Group.
func (s *Service) CreateResourceGroup(options *resourcemanagerv2.CreateResourceGroupOptions) (*resourcemanagerv2.ResCreateResourceGroup, *core.DetailedResponse, error) {
	return s.client.CreateResourceGroup(options)
}

// DeleteResourceGroup deletes the Resource Group.
func (s *Service) DeleteResourceGroup(options *resourcemanagerv2.DeleteResourceGroupOptions) (*core.DetailedResponse, error) {
	return s.client.DeleteResourceGroup(options)
}

// GetResourceGroupByName returns the Resource Group with the provided name, if found.
func (s *Service) GetResourceGroupByName(rgName string) (*resourcemanagerv2.ResourceGroup, error) {
	accountID, err := utils.GetAccountID()
	if err != nil {
		return nil, err
	}

	listOptions := s.client.NewListResourceGroupsOptions()
	listOptions.SetAccountID(accountID)
	listOptions.SetName(rgName)

	result, response, err := s.ListResourceGroups(listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed listing Resource Groups: %w", err)
	}
	if result == nil || result.Resources == nil || len(result.Resources) != 1 || (response != nil && response.StatusCode == http.StatusNotFound) {
		return nil, fmt.Errorf("failed to find Resource Group")
	}
	return &result.Resources[0], nil
}

// GetServiceURL will get the service URL.
func (s *Service) GetServiceURL() string {
	return s.client.GetServiceURL()
}

// SetServiceURL sets the service URL.
func (s *Service) SetServiceURL(url string) error {
	return s.client.SetServiceURL(url)
}

// NewService returns a new service for the IBM Cloud Resource Manager api client.
func NewService(options ServiceOptions) (*Service, error) {
	if options.ResourceManagerV2Options == nil {
		options.ResourceManagerV2Options = &resourcemanagerv2.ResourceManagerV2Options{}
	}
	if options.Authenticator == nil {
		auth, err := authenticator.GetAuthenticator()
		if err != nil {
			return nil, err
		}
		options.Authenticator = auth
	}
	service, err := resourcemanagerv2.NewResourceManagerV2(options.ResourceManagerV2Options)
	if err != nil {
		return nil, err
	}
	return &Service{
		client: service,
	}, nil
}

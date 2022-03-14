package ibmcloud

import (
	"context"
	"fmt"
	"sync"
)

// Metadata holds additional metadata for InstallConfig resources that
// does not need to be user-supplied (e.g. because it can be retrieved
// from external APIs).
type Metadata struct {
	BaseDomain string
	Region     string
	Subnets    []string

	accountID      string
	cisInstanceCRN string
	client         *Client
	privateSubnets map[string]Subnet
	publicSubnets  map[string]Subnet
	vpc            string

	mutex sync.Mutex
}

// NewMetadata initializes a new Metadata object.
func NewMetadata(baseDomain string, region string, subnets []string) *Metadata {
	return &Metadata{BaseDomain: baseDomain, Region: region, Subnets: subnets}
}

// AccountID returns the IBM Cloud account ID associated with the authentication
// credentials.
func (m *Metadata) AccountID(ctx context.Context) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.accountID == "" {
		client, err := m.Client()
		if err != nil {
			return "", err
		}

		apiKeyDetails, err := client.GetAuthenticatorAPIKeyDetails(ctx)
		if err != nil {
			return "", err
		}

		m.accountID = *apiKeyDetails.AccountID
	}
	return m.accountID, nil
}

// CISInstanceCRN returns the Cloud Internet Services instance CRN that is
// managing the DNS zone for the base domain.
func (m *Metadata) CISInstanceCRN(ctx context.Context) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.cisInstanceCRN == "" {
		client, err := m.Client()
		if err != nil {
			return "", err
		}

		zones, err := client.GetDNSZones(ctx)
		if err != nil {
			return "", err
		}

		for _, z := range zones {
			if z.Name == m.BaseDomain {
				m.SetCISInstanceCRN(z.CISInstanceCRN)
				return m.cisInstanceCRN, nil
			}
		}
		return "", fmt.Errorf("cisInstanceCRN unknown due to DNS zone %q not found", m.BaseDomain)
	}
	return m.cisInstanceCRN, nil
}

// SetCISInstanceCRN sets Cloud Internet Services instance CRN to a string value.
func (m *Metadata) SetCISInstanceCRN(crn string) {
	m.cisInstanceCRN = crn
}

// Client returns a client used for making API calls to IBM Cloud services.
func (m *Metadata) Client() (*Client, error) {
	if m.client == nil {
		client, err := NewClient()
		if err != nil {
			return nil, err
		}
		m.client = client
	}
	return m.client, nil
}

// PrivateSubnets retrieves subnet metadata indexed by subnet ID, for
// subnets that the cloud-provider logic considers to be private
// (i.e. not public)
func (m *Metadata) PrivateSubnets(ctx context.Context) (map[string]Subnet, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	err := m.populateSubnets(ctx)
	if err != nil {
		return nil, err
	}

	return m.privateSubnets, nil
}

// PublicSubnets retrieves subnet metadata indexed by subnet ID, for
// subnets that the cloud-provider logic considers to be public
// (e.g. with suitable routing for hosting public load balancers)
func (m *Metadata) PublicSubnets(ctx context.Context) (map[string]Subnet, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	err := m.populateSubnets(ctx)
	if err != nil {
		return nil, err
	}

	return m.publicSubnets, nil
}

// populateSubnets will collect subnets based on metadata and sort them as private or public
func (m *Metadata) populateSubnets(ctx context.Context) (string, map[string]Subnet, map[string]Subnet, err) {
	if len(m.privateSubnets) > 0 || len(m.publicSubnets) > 0 {
		return nil
	}

	if len(m.Subnets) == 0 {
		return errors.New("no subnets configured")
	}

	client, err := m.Client()
	if err != nil {
		return nil
	}

	m.vpc, m.privateSubnets, m.publicSubnets, err := subnets(ctx, client, m.Region, m.Subnets)
	return err
}

// VPC retrieves the VPC Id containing the private and public subnets
func (m *Metadata) VPC(ctx context.Context) (string, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.vpc == "" {
		if len(m.Subnets) == 0 {
			return "", errors.New("cannot calculate VPC without configured subnets")
		}

		err := m.populateSubnets(ctx)
		if err != nil {
			return "", err
		}
	}

	return m.vpc, nil
}

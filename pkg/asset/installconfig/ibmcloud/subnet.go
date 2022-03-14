package ibmcloud

import (
	"context"

	"github.com/pkg/errors"
)

type Subnet struct {
	CIDR string
	CRN  string
	ID   string
	Name string
	VPC  string
	Zone string
}

func getSubnets(ctx context.Context, client *Client, subnetIDs []string) (map[string]Subnet, error) {
	subnets := map[string]Subnet{}

	for _, id := range subnetIDs {
		results, err := client.GetSubnet(ctx, id)
		if err != nil {
			return nil, errors.Wrapf(err, "getting subnet %s", id)
		}

		if results.ID == nil {
			continue
		}

		if results.Ipv4CIDRBlock == nil {
			return nil, errors.Errorf("%s has no Ipv4CIDRBlock", results.ID)
		}

		if results.CRN == nil {
			return nil, errors.Errorf("%s has no CRN", results.ID)
		}

		if results.Name == nil {
			return nil, errors.Errorf("%s has no Name", results.ID)
		}

		if results.VPC == nil {
			return nil, errors.Errorf("%s has no VPC", results.ID)
		}

		if results.Zone == nil {
			return nil, errors.Errorf("%s has no Zone", results.ID)
		}

		subnets[*results.ID] = Subnet{
			CIDR: *results.Ipv4CIDRBlock,
			CRN:  *results.CRN,
			ID:   *results.ID,
			Name: *results.Name,
			VPC:  *results.VPC.Name,
			Zone: *results.Zone.Name,
		}
	}

	return subnets, nil
}

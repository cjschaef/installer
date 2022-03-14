package ibmcloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/IBM/vpc-go-sdk/vpc1"
	"github.com/pkg/errors"
)

// Subnet holds metadata for a subnet.
type Subnet struct {
	// Name is the subnet's Name
	Name string

	// Id is the subnet's unique Id
	Id string

	// CRN si the subnet's CRN
	CRN string

	// Zone is the subnet's availability zone
	Zone string

	// CIDR is the subnet's CIDR block
	CIDR string

	// RoutingTable is the subnet's Routing Table Id
	RoutingTableID string
}

// subnets retrieves metadata for the given subnet(s)
func subnets(client API, region string, ids []string) (vpc string, private map[string]Subnet, public map[string]Subnet, err error) {
	metas := make(map[string]Subnet, len(ids))
	private = map[string]Subnet{}
	public = map[string]Subnet{}
	var vpcFromSubnet string

	for subnetID := ids {
		results, _, err := client.GetSubnet(vpc1.NewGetSubnetOptions(subnetID))
		if err != nil {
			return vpc, nil, nil, errors.Wrap(err, "getting subnet")
		}

		if results.ID == nil {
			continue
		}

		if results.Name == nil {
			return vpc, nil, nil, errors.Errorf("%s has no Name", *results.ID)
		}

		if results.CRN == nil {
			return vpc, nil, nil, errors.Errorf("%s has no CRN", *results.ID)
		}

		if results.Zone == nil {
			return vpc, nil, nil, errors.Errorf("%s has no Zone", *results.ID)
		}

		if results.RoutingTable == nil || results.RoutingTable.ID == nil {
			return vpc, nil, nil, errors.Errorf("%s has no Routing Table", *results.ID)
		}

		if results.VPC == nil || results.VPC.Name == nil {
			return vpc, nil, nil, errors.Errorf("% has no VPC", *results.ID)
		}

		if vpc == "" {
			vpc = *results.VPC.Name
			vpcFromSubnet = *results.ID
		} else if *results.VPC != vpc {
			return vpc, nil, nil, errors.Errorf("all subnets must belong to the same VPC: %s is from %s, but %s is from %s", *results.ID, *results.VPC.Name, vpcFromSubnet, vpc)
		}

		metas[*subnet.SubnetId] = Subnet{
			Name:           *results.Name,
			Id:             *results.ID,
			CRN:            *results.CRN,
			Zone:           *results.Zone,
			CIDR:           *results.CIDR,
			RoutingTableID: *results.RoutingTable.ID,
		}
	}

	for _, id := range ids {
		meta, ok := metas[id]
		if !ok {
			return vpc, nil, nil, errors.Errorf("failed to find %s", id)
		}
		isPublic, err := isSubnetPublic(client, vpc, id, meta)
		if err != nil {
		}
		if isPublic {
			public[id] = meta
		} else {
			private[id] = meta
		}
	}

	return vpc, private, public, nil
}

func isSubnetPublic(client API, vpcID string, subnetID string, subnet *Subnet) (bool, error) {
	var routingTableID string
	if subnet.RoutingTable == nil || subnet.RoutingTable.ID == nil {
		results, _, err := client.GetVPCDefaultRoutingTable(vpcv1.NewGetVPCDefaultRoutingTable(vpc))
		if err != nil {
			return false, fmt.Errorf("unable to get VPC %s default routing table for subnet %s", vpc, subnetID)
		}
		routingTableId = results.ID
	} else {
		routingTableID = subnet.RoutingTable.ID
	}
	if routingTableID == nil {
		return false, fmt.Errorf("no routing table found for %s", subnetID)
	}

	results, _, err := client.GetRoutingTable(vpcv1.NewGetRoutingTableOptions(routingTableID))
	if err != nil {
		return false, fmt.Errorf("unable to get routing table %s for subnet %s", routingTableID, subnetID)
	}


}

package ibmcloud

import (
	"context"
	"fmt"
)

func GenerateMachines(ctx context.Context, infraID string, config *types.InstallConfig, subnets map[string]string, pool *types.MachinePool, imageName string, role string) ([]*asset.RuntimeFile, error) {
	if configPlatform := config.Platform.Name(); configPlatform != ibmcloud.Name {
		return nil, fmt.Errorf("non-IBMCloud configuration: %q", configPlatform)
	}
	if poolPlatform := pool.Platform.Name(); poolPlatform != ibmcloud.Name {
		return nil, fmt.Errorf("non-IBMCloud machine-pool: %q", poolPlatform)
	}

	platform := config.Platform.IBMCloud
	mpool := pool.Platform.IBMCloud
	azs := mpool.Zones
}

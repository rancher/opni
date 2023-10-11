package codegen

import (
	"reflect"

	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

func GenCortexConfig() error {
	if err := generate[bucket.Config]("github.com/rancher/opni/internal/cortex/config/storage/storage.proto"); err != nil {
		return err
	}
	if err := generate[validation.Limits]("github.com/rancher/opni/internal/cortex/config/validation/limits.proto"); err != nil {
		return err
	}
	if err := generate[cortex.RuntimeConfigValues]("github.com/rancher/opni/internal/cortex/config/runtimeconfig/runtimeconfig.proto"); err != nil {
		return err
	}
	if err := generate[compactor.Config]("github.com/rancher/opni/internal/cortex/config/compactor/compactor.proto"); err != nil {
		return err
	}
	if err := generate[querier.Config]("github.com/rancher/opni/internal/cortex/config/querier/querier.proto",
		func(rf reflect.StructField) bool {
			if rf.Name == "StoreGatewayAddresses" || rf.Name == "StoreGatewayClient" {
				return true
			}
			return false
		},
	); err != nil {
		return err
	}

	return nil
}

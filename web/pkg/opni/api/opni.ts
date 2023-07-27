import * as CortexOpsService from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_svc';
import * as CortexOpsTypes from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_pb';
import * as StorageTypes from '@pkg/opni/generated/github.com/rancher/opni/internal/cortex/config/storage/storage_pb';
import * as DryRunTypes from '@pkg/opni/generated/github.com/rancher/opni/pkg/plugins/driverutil/dryrun_pb';

export const CortexOps = {
  service: CortexOpsService,
  types:   CortexOpsTypes,
};

export const DryRun = { types: DryRunTypes };

export const Storage = { types: StorageTypes };

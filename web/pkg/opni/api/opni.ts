import * as CortexOpsService from '../generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_svc';
import * as CortexOpsTypes from '../generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_pb';
import * as StorageTypes from '../generated/github.com/rancher/opni/internal/cortex/config/storage/storage_pb';
import * as ManagementService from '../generated/github.com/rancher/opni/pkg/apis/management/v1/management_svc';
import * as ManagementTypes from '../generated/github.com/rancher/opni/pkg/apis/management/v1/management_pb';
import * as CoreTypes from '../generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import * as DryRunTypes from '../generated/github.com/rancher/opni/pkg/plugins/driverutil/dryrun_pb';

export const CortexOps = {
  service: CortexOpsService,
  types:   CortexOpsTypes,
};

export const Management = {
  service: ManagementService,
  types:   ManagementTypes,
};

export const DryRun = { types: DryRunTypes };

export const Storage = { types: StorageTypes };

export const Core = { types: CoreTypes };

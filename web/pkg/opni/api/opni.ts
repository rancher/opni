import * as CortexOpsService from '../generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_svc';
import * as CortexOpsTypes from '../generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_pb';
import * as StorageTypes from '../generated/github.com/rancher/opni/internal/cortex/config/storage/storage_pb';
import * as ManagementService from '../generated/github.com/rancher/opni/pkg/apis/management/v1/management_svc';
import * as ManagementTypes from '../generated/github.com/rancher/opni/pkg/apis/management/v1/management_pb';
import * as CoreTypes from '../generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import * as DriverUtilTypes from '../generated/github.com/rancher/opni/pkg/plugins/driverutil/types_pb';
import * as CapabilityTypes from '../generated/github.com/rancher/opni/pkg/apis/capability/v1/capability_pb';

export const CortexOps = {
  service: CortexOpsService,
  types:   CortexOpsTypes,
};

export const Management = {
  service: ManagementService,
  types:   ManagementTypes,
};

export const DriverUtil = { types: DriverUtilTypes };

export const Storage = { types: StorageTypes };

export const Core = { types: CoreTypes };

export const Capability = { types: CapabilityTypes };

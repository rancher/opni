import * as CortexOpsService from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_svc';
import * as CortexOpsTypes from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/cortexops/cortexops_pb';
import * as StorageTypes from '@pkg/opni/generated/github.com/rancher/opni/internal/cortex/config/storage/storage_pb';
import * as ManagementService from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/management/v1/management_svc';
import * as ManagementTypes from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/management/v1/management_pb';
import * as CoreTypes from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import * as DriverUtilTypes from '@pkg/opni/generated/github.com/rancher/opni/pkg/plugins/driverutil/types_pb';
import * as CapabilityTypes from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/capability/v1/capability_pb';
import * as LoggingAdminService from '@pkg/opni/generated/github.com/rancher/opni/plugins/logging/apis/loggingadmin/loggingadmin_svc';
import * as LoggingAdminTypes from '@pkg/opni/generated/github.com/rancher/opni/plugins/logging/apis/loggingadmin/loggingadmin_pb';

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

export namespace LoggingAdmin {
  export import Service = LoggingAdminService;
  export import Types = LoggingAdminTypes;
}

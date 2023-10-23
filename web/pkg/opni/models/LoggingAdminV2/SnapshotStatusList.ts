import { LoggingAdmin } from '@pkg/opni/api/opni';
import { SnapshotStatus } from './SnapshotStatus';

export class SnapshotStatusList {
    base: LoggingAdmin.Types.SnapshotStatusList;

    constructor(base: LoggingAdmin.Types.SnapshotStatusList) {
      this.base = base;
    }

    get statuses(): SnapshotStatus[] {
      return this.base.statuses.map(s => new SnapshotStatus(s));
    }
}

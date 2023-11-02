import { LoggingAdmin } from '@pkg/opni/api/opni';
import { Resource } from '@pkg/opni/models/Resource';
import dayjs from 'dayjs';

export class SnapshotStatus extends Resource {
    base: LoggingAdmin.Types.SnapshotStatus;

    constructor(base: LoggingAdmin.Types.SnapshotStatus) {
      super(null);

      this.base = base;
    }

    get nameDisplay(): string {
      return this.base.ref?.name || '';
    }

    get id(): string {
      return this.nameDisplay;
    }

    get lastUpdated(): string {
      const format = 'MM-DD-YY (h:mm:ss a)';

      return this.base.lastUpdated ? dayjs(Number(this.base.lastUpdated?.seconds)).format(format) : 'Unknown';
    }

    get status() {
      switch (this.base.status) {
      case 'Success':
        return {
          state:   'success',
          message: 'Success'
        };
      case 'In Progress':
        return {
          state:   'info',
          message: 'In Progress'
        };
      case 'Retrying':
        return {
          state:   'warning',
          message: 'Retrying'
        };
      case 'Failed':
        return {
          state:   'error',
          message: 'Failed'
        };
      case 'Timed Out':
        return {
          state:   'error',
          message: 'Timed Out'
        };
      default:
        return {
          state:   'warning',
          message: `Unknown${ this.base.status ? ` - ${ this.base.status }` : '' }`
        };
      }
    }

    get availableActions(): any[] {
      return [
        {
          action:    'edit',
          altAction: 'edit',
          label:     'Edit',
          icon:      'icon icon-edit',
          enabled:   true,
        },
        {
          action:     'promptRemove',
          altAction:  'delete',
          label:      'Delete',
          icon:       'icon icon-trash',
          bulkable:   true,
          enabled:    true,
          bulkAction: 'promptRemove',
          weight:     -10, // Delete always goes last
        }
      ];
    }

    edit() {
      this.changeRoute({
        name:   'snapshot',
        params: { id: this.id }
      });
    }

    async remove() {
      await LoggingAdmin.Service.DeleteSnapshotSchedule(new LoggingAdmin.Types.SnapshotReference({ name: this.id }));
      super.remove();
    }
}

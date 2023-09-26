import { deleteRoleBinding } from '../utils/requests/management';
import { Resource } from './Resource';

export interface RoleBindingResponse {
  id: string;
  roleId: string;
  subjects: string[];
  metadata: Metadata;
  taints: string[];
}

export interface Metadata {
  capability: string;
}

export interface RoleBindingsResponse {
  items: RoleBindingResponse[];
}

export class RoleBinding extends Resource {
    private base: RoleBindingResponse;

    constructor(base: RoleBindingResponse, vue: any) {
      super(vue);
      this.base = base;
    }

    get name() {
      return this.base.id;
    }

    get nameDisplay(): string {
      return this.name;
    }

    get subjects() {
      return this.base.subjects;
    }

    get role() {
      return this.base.roleId;
    }

    get taints() {
      return this.base.taints;
    }

    get availableActions(): any[] {
      return [
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

    async remove() {
      await deleteRoleBinding(this.base.id);
      super.remove();
    }
}

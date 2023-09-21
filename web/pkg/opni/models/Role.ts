import { deleteRole } from '../utils/requests/management';
import { findBy } from '../utils/array';
import { Resource } from './Resource';

export interface MatchExpression {
    key: string;
    operator: string;
    values: string[]
}

export interface MatchLabel {
    matchLabels: { [key: string]: string };
    matchExpressions: MatchExpression[]
}

export interface RoleResponse {
  id: string;
  clusterIDs: string[];
  matchLabels: MatchLabel
}

export interface RolesResponse {
  items: RoleResponse[];
}

export class Role extends Resource {
    private base: RoleResponse;

    constructor(base: RoleResponse, vue: any) {
      super(vue);
      this.base = base;
    }

    get id() {
      return this.base.id;
    }

    get name() {
      return this.id;
    }

    get nameDisplay(): string {
      return this.name;
    }

    get clusterIds() {
      return this.base.clusterIDs;
    }

    get clusters() {
      return this.vue.$store.getters['opni/clusters'];
    }

    get clusterNames() {
      if (!this.clusters) {
        throw new Error('You must call setClusters to use clusterNames.');
      }

      return this.clusterIds
        .map((clusterId) => {
          const cluster = findBy(this.clusters || [], 'id', clusterId);

          return cluster?.nameDisplay;
        })
        .filter(n => n);
    }

    get matchExpressionsDisplay() {
      return this.base.matchLabels.matchExpressions.map(this.formatMatchExpression);
    }

    formatMatchExpression(matchExpression: MatchExpression) {
      const values = matchExpression.values.length > 0 ? ` [${ matchExpression.values.join(', ') }]` : '';
      const operator = matchExpression.operator.toUpperCase();

      return `${ matchExpression.key } ${ operator }${ values }`;
    }

    get matchLabelsDisplay() {
      return Object.entries(this.base.matchLabels.matchLabels).map(([key, value]) => `${ key }=${ value }`);
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
      await deleteRole(this.base.id);
      super.remove();
    }
}

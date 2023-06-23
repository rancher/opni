import { Resource } from './Resource';

export interface DeploymentResponse {
  clusterId: string;
  logCount: string;
  deployment: string;
  namespace: string;
 }

export class Deployment extends Resource {
  base: DeploymentResponse;

  constructor(base: DeploymentResponse, vue: any) {
    super(vue);

    this.base = base;
  }

  get clusterId(): string {
    return this.base.clusterId;
  }

  get id(): string {
    return [this.base.clusterId, this.base.namespace, this.base.deployment].join('_');
  }

  get nameDisplay(): string {
    return this.base.deployment;
  }

  get namespace(): string {
    return this.base.namespace;
  }

  get logs(): number {
    return Number.parseInt(this.base.logCount);
  }

  get availableActions(): any[] {
    return [
      {
        action:     'queue',
        label:      'Queue',
        icon:       'icon icon-move',
        bulkable:   true,
        enabled:    true,
        bulkAction: 'queue',
        weight:     -10, // Delete always goes last
      }
    ];
  }
}

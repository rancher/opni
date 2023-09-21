import Vue, { reactive } from 'vue';
import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';
import { deleteCluster } from '@pkg/opni/utils/requests/management';
import * as Core from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import { LABEL_KEYS } from './shared';
import { Resource } from './Resource';
import { TaskState, Capability } from './Capability';

export interface ClusterResponse {
  id: string;
  metadata: {
    labels: { [key: string]: string };
    capabilities: {
      name: string;
      deletionTimestamp?: string;
    }[];
  }
}

export interface HealthResponse {
  health: {
    ready: boolean;
    conditions: string[];
  },
  status: {
    timestamp: string;
    connected: boolean;
    sessionAttributes: string[];
  }
}

export interface ClustersResponse {
  items: ClusterResponse[];
}

export interface ClusterStats {
  userID: string;
  ingestionRate: number;
  numSeries: number;
  APIIngestionRate: number;
  RuleIngestionRate: number;
}

export interface ClusterStatsList {
  items: ClusterStats[];
}

export interface CapabilityLog {
  capability: string;
  state: string;
  message: string;
}

export interface CapabilityStatusLogResponse {
  msg: string;
  level: number;
  timestamp: string;
}

export interface CapabilityStatusTransitionResponse {
  state: string;
  timestamp: string;
}

export interface CapabilityStatusResponse {
  state: TaskState;
  progress: null;
  metadata: 'string';
  logs: CapabilityStatusLogResponse[];
  transitions: CapabilityStatusTransitionResponse[];
}

export type CapabilityStatusState = 'info' | 'success' | 'warning' | 'error' | null;
export interface CapabilityStatus {
  state: CapabilityStatusState;
  message: string;
  pending: boolean;
}

export interface CapabilityStatuses {
  metrics?: CapabilityStatus;
  logging?: CapabilityStatus;
}

export class Cluster extends Resource {
  private base?: Core.Cluster;
  private healthStatus?: Core.HealthStatus;
  private clusterStats: ClusterStats;

  constructor(vue: any) {
    super(vue);
    this.clusterStats = {
      ingestionRate: 0,
      numSeries:     0,
    } as ClusterStats;
  }

  static create(vue: any): Cluster {
    return reactive(new Cluster(vue));
  }

  get status() {
    if (!this.healthStatus) {
      return {
        state:   '',
        message: 'Unknown'
      };
    }
    if (!this.healthStatus?.status?.connected) {
      return {
        state:   'error',
        message: 'Disconnected'
      };
    }

    if (!this.healthStatus?.health?.ready) {
      return {
        state:        'warning',
        shortMessage: 'Degraded',
        message:      this.healthStatus?.health?.conditions.join(', ')
      };
    }

    return {
      state:   'success',
      message: 'Ready'
    };
  }

  get isLocal(): boolean {
    return this.healthStatus?.status?.sessionAttributes?.includes('local') || false;
  }

  get localIcon(): string {
    return this.isLocal ? 'icon-checkmark text-success' : '';
  }

  get type(): string {
    return 'cluster';
  }

  get nameDisplay(): string {
    return this.name || this.base?.id || '';
  }

  get name(): string {
    return this.base?.metadata?.labels[LABEL_KEYS.NAME] || '';
  }

  get id(): string {
    return this.base?.id || '';
  }

  get labels(): { [key: string]: string } {
    return this.base?.metadata?.labels || {};
  }

  get visibleLabels(): { [key: string]: string } {
    const labels: any = {};

    Object.entries(this.base?.metadata?.labels || {})
      .filter(([key]) => !key.includes('opni.io'))
      .forEach(([key, value]) => {
        labels[key] = value;
      });

    return labels;
  }

  get hiddenLabels(): any {
    const labels: any = {};

    Object.entries(this.base?.metadata?.labels || {})
      .filter(([key]) => key.includes('opni.io'))
      .forEach(([key, value]) => {
        labels[key] = value;
      });

    return labels;
  }

  get displayLabels(): string[] {
    return Object.entries(this.visibleLabels)
      .map(([key, value]) => `${ key }=${ value }`);
  }

  get capabilities(): string[] {
    return this.base?.metadata?.capabilities?.map(capability => capability.name) || [];
  }

  get capabilityModels(): Capability[] {
    return this.base?.metadata?.capabilities?.map(capability => Capability.create(capability.name as any, this, this.vue) ) || [];
  }

  get capabilitiesRaw(): Core.ClusterCapability[] {
    return this.base?.metadata?.capabilities || [];
  }

  isCapabilityInstalled(type: string) {
    return this.capabilities.includes(type);
  }

  get nodes(): [] {
    return [];
  }

  get stats(): ClusterStats {
    return this.clusterStats;
  }

  set stats(stats: ClusterStats) {
    this.clusterStats = stats;
  }

  onClusterUpdated(cluster: Core.Cluster) {
    Vue.set(this, 'base', cluster);
  }

  onHealthStatusUpdated(healthStatus: Core.HealthStatus) {
    Vue.set(this, 'healthStatus', healthStatus);
  }

  uninstallCapabilities() {
    GlobalEventBus.$emit('uninstallCapabilities', this.capabilityModels);
  }

  get availableActions(): any[] {
    return [
      {
        action:   'promptEdit',
        label:    'Edit',
        icon:     'icon icon-edit',
        bulkable: false,
        enabled:  true,
      },
      {
        action:   'copy',
        label:    'Copy ID',
        icon:     'icon icon-copy',
        bulkable: false,
        enabled:  true,
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

  get version() {
    return this.hiddenLabels[LABEL_KEYS.VERSION] || 'v1';
  }

  async remove() {
    if (!this.base?.id) {
      return;
    }
    await deleteCluster(this.base.id);
    super.remove();
  }

  public promptRemove(resources = this) {
    if (this.capabilities.length > 0) {
      GlobalEventBus.$emit('cantDeleteCluster', this);
    } else {
      GlobalEventBus.$emit('promptRemove', resources);
    }
  }
}

import Vue from 'vue';
import { getCapabilityStatus, installCapabilityV2, uninstallCapabilityStatus } from '../utils/requests/management';
import { exceptionToErrorsArray } from '../utils/error';
import { Cluster } from './Cluster';
import { Resource } from './Resource';

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

export enum CapabilityStatusState {
  Unknown = 0,
  Success = 1,
  Warning = 2,
  Error = 3,
}

export interface CapabilityStatusResponse {
  state: CapabilityStatusState;
  progress: null;
  metadata: 'string';
  logs: CapabilityStatusLogResponse[];
  transitions: CapabilityStatusTransitionResponse[];
}

export enum TaskState {
  Unknown = 0,
  Pending = 1,
  Running = 2,
  Completed = 3,
  Failed = 4,
  Canceled = 6,
}

export interface NodeCapabilityStatus {
  enabled: boolean;
  lastSync?: string;
  conditions?: string[];
}

export interface CapabilityStatus {
  state: string;
  shortMessage: string;
  message: string;
}

export interface ClusterStats {
  userID: string;
  ingestionRate: number;
  numSeries: number;
  APIIngestionRate: number;
  RuleIngestionRate: number;
}

export interface CapabilityStatuses {
  metrics?: CapabilityStatus;
  logs?: CapabilityStatus;
}
export class Capability extends Resource {
  private type: keyof CapabilityStatuses;
  private cluster: Cluster;
  private capLogs: CapabilityLog[];
  private capabilityStatus: CapabilityStatuses;
  private stats?: ClusterStats[];

  constructor(type: keyof CapabilityStatuses, cluster: Cluster, vue: any) {
    super(vue);
    this.type = type;
    this.cluster = cluster;
    this.capLogs = [];
    this.capabilityStatus = {};
    Vue.set(this, 'capabilityStatus', {});
  }

  get rawCluster() {
    return this.cluster;
  }

  get rawType() {
    return this.type;
  }

  get id() {
    return this.cluster.id;
  }

  get status() {
    const status = this.capabilityStatus[this.type];

    return status || {
      state:   'info',
      message: 'Not Installed',
    };
  }

  get isInstalled() {
    return this.cluster.capabilities?.includes(this.type);
  }

  get isLocal(): boolean {
    return this.cluster.isLocal;
  }

  get localIcon(): string {
    return this.isLocal ? 'icon-checkmark text-success' : '';
  }

  get clusterNameDisplay(): string {
    return this.cluster.nameDisplay;
  }

  get capabilities(): string[] {
    return this.cluster.capabilities;
  }

  get capabilityLogs(): CapabilityLog[] {
    return this.capLogs;
  }

  updateCapabilities(): Promise<void> {
    return this.cluster.updateCapabilities();
  }

  async updateCabilityLogs(): Promise<void> {
    const logs: CapabilityLog[] = [];

    function getState(state: TaskState) {
      switch (state) {
      case TaskState.Completed:
        return null;
      case TaskState.Running:
      case TaskState.Pending:
      case TaskState.Canceled:
        return 'warning';
      default:
        return 'error';
      }
    }

    for (const i in this.capabilities) {
      try {
        const capability = this.capabilities[i] as (keyof CapabilityStatuses);
        const capMeta = this.cluster.capabilitiesRaw?.find(c => c.name === capability);

        if (!capMeta) {
          continue;
        }

        if (!capMeta.deletionTimestamp) {
          const apiStatus = await getCapabilityStatus(this.cluster.id, capability, this.vue);

          if (apiStatus.conditions?.length === 0) {
            Vue.set(this.capabilityStatus, capability, {
              state:   'success',
              message: 'Installed',
            });
          } else if (!apiStatus.enabled) {
            Vue.set(this.capabilityStatus, capability, {
              state:   'warning',
              message: 'Disabled',
            });
          } else {
            Vue.set(this.capabilityStatus, capability, {
              state:        'warning',
              shortMessage: 'Degraded',
              message:      apiStatus.conditions?.join(', '),
            });
          }
        } else {
          const log = await uninstallCapabilityStatus(this.cluster.id, capability, this.vue);
          const pending = log.state === TaskState.Pending || log.state === TaskState.Running || (this.capabilityStatus[capability] as any )?.pending || false;
          const state = getState(log.state);

          Vue.set(this.capabilityStatus, capability, {
            state,
            message: state === null ? '' : (log.logs || []).reverse()[0]?.msg,
            pending
          });
        }
      } catch (ex) { }
    }

    this.capLogs = logs;

    await this.updateCapabilities();
  }

  get availableActions(): any[] {
    return [
      {
        action:   'install',
        label:    'Install',
        icon:     'icon icon-upload',
        bulkable: true,
        enabled:  !this.isCapabilityInstalled,
      },
      {
        action:   'uninstall',
        label:    'Uninstall',
        icon:     'icon icon-delete',
        bulkable: true,
        enabled:  this.isCapabilityInstalled,
      },
      {
        action:   'cancelUninstall',
        label:    'Cancel Uninstall',
        icon:     'icon icon-x',
        bulkable: true,
        enabled:  this.isCapabilityUninstalling,
      }
    ];
  }

  uninstall() {
    this.vue.$emit('uninstallCapabilities', [this]);
  }

  cancelUninstall() {
    this.vue.$emit('cancelUninstallCapabilities', [this]);
  }

  get isCapabilityInstalled() {
    return this.capabilities.includes(this.type);
  }

  async install() {
    try {
      const result = await installCapabilityV2(this.type, this.cluster.id);

      Vue.set(this.capabilityStatus, this.type, {
        state:   CapabilityStatusState[result.status].toLowerCase(),
        message: result.status === CapabilityStatusState.Success ? 'Installed' : `Installation problem: ${ result.message }`,
      });

      await this.updateCapabilities();
    } catch (ex) {
      Vue.set(this.capabilityStatus, this.type, {
        state:   'error',
        message: exceptionToErrorsArray(ex).join('; '),
      });
    }
  }

  clearCapabilityStatus(capabilities: (keyof CapabilityStatuses)[]) {
    capabilities.forEach((capability) => {
      Vue.set(this.capabilityStatus, capability, {
        state:   null,
        message: null,
      });
    });
  }

  get isCapabilityUninstalling(): boolean {
    const cap = this.cluster.capabilitiesRaw?.find(c => c.name === this.type);

    return !!(cap?.deletionTimestamp);
  }

  get numSeries(): number | undefined {
    return this.clusterStats?.numSeries;
  }

  get sampleRate(): number | undefined {
    return Math.floor(this.clusterStats?.ingestionRate || 0);
  }

  get rulesRate(): number | undefined {
    return this.clusterStats?.RuleIngestionRate;
  }

  get clusterStats(): ClusterStats | undefined {
    return this.stats?.find(s => s.userID === this.cluster.id);
  }

  updateStats(stats: ClusterStats[]) {
    this.stats = stats;
  }
}

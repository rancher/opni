import Vue from 'vue';
import { proto3 } from '@bufbuild/protobuf';
import { getCapabilityStatus, installCapabilityV2, uninstallCapabilityStatus } from '../utils/requests/management';
import { exceptionToErrorsArray } from '../utils/error';
import { CortexOps, Management } from '../api/opni';
import { CapabilityInstallRequest, CapabilityStatusRequest } from '../generated/github.com/rancher/opni/pkg/apis/management/v1/management_pb';
import { Reference } from '../generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import { InstallRequest, InstallResponseStatus } from '../generated/github.com/rancher/opni/pkg/apis/capability/v1/capability_pb';
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
  alerting?: CapabilityStatus;
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

  get nameDisplay(): string {
    return {
      metrics: 'Monitoring', logs: 'Logging', alerting: 'Alerting'
    }[this.rawType];
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
      state:        'info',
      shortMessage: 'Not Installed',
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

  // updateCapabilities(): Promise<void> {
  //   return this.cluster.updateCapabilities();
  // }

  async updateCabilityLogs(): Promise<void> {
    const logs: CapabilityLog[] = [];

    function getState(state: TaskState) {
      switch (state) {
      case TaskState.Completed:
        return 'info';
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
          const apiStatus = await Management.service.CapabilityStatus(new CapabilityStatusRequest({
            cluster: new Reference({ id: this.cluster.id }),
            name:    capability,
          }));

          if (apiStatus.conditions?.length === 0) {
            Vue.set(this.capabilityStatus, capability, {
              state:        'success',
              shortMessage: 'Installed',
            });
          } else if (!apiStatus.enabled) {
            Vue.set(this.capabilityStatus, capability, {
              state:        'warning',
              shortMessage: 'Disabled',
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
            shortMessage: pending ? 'Pending' : (log.state === TaskState.Completed ? 'Not Installed' : 'Uninstalling'),
            message:      (log.logs || []).reverse()[0]?.msg,
          });
        }
      } catch (ex) {}
    }

    this.capLogs = logs;

    // await this.updateCapabilities();
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
      const result = await Management.service.InstallCapability(new CapabilityInstallRequest({
        name:   this.type,
        target: new InstallRequest({ cluster: new Reference({ id: this.cluster.id }) }),
      }));

      Vue.set(this.capabilityStatus, this.type, {
        state:        InstallResponseStatus[result.status].toLowerCase(),
        shortMessage: result.status === InstallResponseStatus.Success ? 'Installed' : InstallResponseStatus[result.status],
        message:      result.message,
      });

      // await this.updateCapabilities();
    } catch (ex) {
      Vue.set(this.capabilityStatus, this.type, {
        state:        'error',
        shortMessage: 'Error',
        message:      exceptionToErrorsArray(ex).join('; '),
      });
    }
  }

  clearCapabilityStatus(capabilities: (keyof CapabilityStatuses)[]) {
    capabilities.forEach((capability) => {
      Vue.set(this.capabilityStatus, capability, {});
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

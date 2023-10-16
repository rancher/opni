import Vue, { reactive } from 'vue';
import * as core from '@pkg/opni/generated/github.com/rancher/opni/pkg/apis/core/v1/core_pb';
import * as node from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/node/config_pb';
import * as nodesvc from '@pkg/opni/generated/github.com/rancher/opni/plugins/metrics/apis/node/config_svc';

import { Cluster } from './Cluster';
import { Capability } from './Capability';

export class MetricCapability extends Capability {
  config?: node.MetricsCapabilityConfig

  constructor(cluster: Cluster, vue: any) {
    super('metrics', cluster, vue);

    this.fetchConfig();
  }

  async fetchConfig() {
    try {
      Vue.set(this, 'config', await nodesvc.GetConfiguration(new node.GetRequest({ node: new core.Reference({ id: this.id }) })));
    } catch (e) {
    }
  }

  static createExtended(cluster: Cluster, vue: any): MetricCapability {
    return reactive(new MetricCapability(cluster, vue));
  }

  get provider(): string {
    if (this.config?.driver) {
      // newer versions set the driver name explicitly
      return node.MetricsCapabilityConfig_Driver[this.config.driver];
    }
    // older versions used a oneof
    if (this.config?.prometheus) {
      return 'Prometheus';
    }
    if (this.config?.otel) {
      return 'OpenTelemetry';
    }

    return '—';
  }

  get availableActions(): any[] {
    return [
      ...super.availableActions,
      {
        action:   'usePrometheus',
        label:    'Use Prometheus',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.provider === 'OpenTelemetry',
      },
      {
        action:   'useOpenTelemetry',
        label:    'Use OpenTelemetry',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.provider === 'Prometheus',
      }
    ];
  }

  get status() {
    const st = super.status;

    if (st.state === 'info' && this.isLocal && !this.isInstalled) {
      return {
        state:        'warning',
        shortMessage: 'Degraded',
        message:      'The local agent should have the capability installed. Without it, some of the default Grafana dashboards may be degraded.'
      };
    }

    return st;
  }

  async usePrometheus() {
    await this.fetchConfig();
    if (this.config) {
      await nodesvc.SetConfiguration(new node.SetRequest({
        node: new core.Reference({ id: this.id }),
        spec: {
          ...structuredClone(this.config),
          driver: node.MetricsCapabilityConfig_Driver.Prometheus,
        },
      }));
      await this.fetchConfig();
    } else {

    }
  }

  async useOpenTelemetry() {
    await this.fetchConfig();
    if (this.config) {
      await nodesvc.SetConfiguration(new node.SetRequest({
        node: new core.Reference({ id: this.id }),
        spec: {
          ...structuredClone(this.config),
          driver: node.MetricsCapabilityConfig_Driver.OpenTelemetry,
        },
      }));
      await this.fetchConfig();
    } else {

    }
  }
}

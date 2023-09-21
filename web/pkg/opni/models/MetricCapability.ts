import Vue, { reactive } from 'vue';
import { Provider, setNodeConfiguration, getNodeConfiguration } from '../utils/requests/node-configuration';
import { Cluster } from './Cluster';
import { Capability } from './Capability';

export class MetricCapability extends Capability {
  providerRaw?: Provider;

  constructor(cluster: Cluster, vue: any) {
    super('metrics', cluster, vue);

    this.updateProvider();
  }

  updateProvider() {
    getNodeConfiguration(this.id).then((config) => {
      Vue.set(this, 'providerRaw', config.prometheus ? 'Prometheus' : 'OpenTelemetry');
    }, () => {
      Vue.set(this, 'providerRaw', 'OpenTelemetry');
    });
  }

  static createExtended(cluster: Cluster, vue: any): MetricCapability {
    return reactive(new MetricCapability(cluster, vue));
  }

  get provider(): string {
    return this.isInstalled && this.providerRaw ? this.providerRaw : '—';
  }

  get availableActions(): any[] {
    return [
      ...super.availableActions,
      {
        action:   'usePrometheus',
        label:    'Use Prometheus',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.isInstalled && this.provider === 'OpenTelemetry',
      },
      {
        action:   'useOpenTelemetry',
        label:    'Use OpenTelemetry',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.isInstalled && this.provider === 'Prometheus',
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
    Vue.set(this, 'providerRaw', 'Prometheus');
    await setNodeConfiguration(this.id, 'Prometheus');
    this.updateProvider();
  }

  async useOpenTelemetry() {
    Vue.set(this, 'providerRaw', 'OpenTelemetry');
    await setNodeConfiguration(this.id, 'OpenTelemetry');
    this.updateProvider();
  }
}

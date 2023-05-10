import Vue from 'vue';
import { Provider, setNodeConfiguration } from '../utils/requests/node-configuration';
import { Cluster } from './Cluster';
import { Capability } from './Capability';

export class MetricCapability extends Capability {
  providerRaw: Provider;

  constructor(cluster: Cluster, provider: Provider, vue: any) {
    super('metrics', cluster, vue);

    this.providerRaw = provider;
  }

  get provider(): string {
    return this.isInstalled ? this.providerRaw : '—';
  }

  get availableActions(): any[] {
    return [
      ...super.availableActions,
      {
        action:   'usePrometheus',
        label:    'Use Prometheus',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.isInstalled && this.provider !== 'Prometheus',
      },
      {
        action:   'useOpenTelemetry',
        label:    'Use OpenTelemetry',
        icon:     'icon icon-fork',
        bulkable: true,
        enabled:  this.isInstalled && this.provider !== 'OpenTelemetry',
      }
    ];
  }

  usePrometheus() {
    setNodeConfiguration(this.id, 'Prometheus');
    Vue.set(this, 'providerRaw', 'Prometheus');
  }

  useOpenTelemetry() {
    setNodeConfiguration(this.id, 'OpenTelemetry');
    Vue.set(this, 'providerRaw', 'OpenTelemetry');
  }
}

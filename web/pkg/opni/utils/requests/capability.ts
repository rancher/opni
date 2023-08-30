import { MetricCapability } from '../../models/MetricCapability';
import { Capability } from '../../models/Capability';
import { getClusters } from './management';
import { getNodeConfiguration } from './node-configuration';

export async function getLoggingCapabilities(vue: any): Promise<Capability[]> {
  const clusters = vue.$store.getters.clusters;

  return (await clusters).map(c => new Capability('logs', c, vue));
}

export async function getMetricCapabilities(vue: any): Promise<Capability[]> {
  const clusters = await getClusters(vue);
  const configurations = await Promise.all(clusters.map(c => getNodeConfiguration(c.id)));

  return (await clusters).map((c, i) => {
    const config = configurations[i];
    const provider = config?.prometheus ? 'Prometheus' : 'OpenTelemetry';

    return new MetricCapability(c, provider, vue);
  });
}

export async function getAlertingCapabilities(vue: any): Promise<Capability[]> {
  const clusters = getClusters(vue);

  return (await clusters).map(c => new Capability('alerting', c, vue));
}

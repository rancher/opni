import { MetricCapability } from '../../models/MetricCapability';
import { Capability } from '../../models/Capability';
import { getClusters } from './management';
import { getNodeConfiguration, setNodeConfiguration } from './node-configuration';

export async function getLoggingCapabilities(vue: any): Promise<Capability[]> {
  const clusters = getClusters(vue);

  (await clusters).forEach(c => setNodeConfiguration(c.id, 'Prometheus'));

  return (await clusters).map(c => new Capability('logging', c, vue));
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

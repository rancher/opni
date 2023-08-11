import axios from 'axios';

export type Provider = 'Prometheus' | 'OpenTelemetry';

export async function getNodeConfiguration(id: string): Promise<any> {
  return (await axios.get(`opni-api/NodeConfiguration/node_config/${ id }`)).data;
}

export async function setNodeConfiguration(id: string, provider: Provider): Promise<any> {
  const spec = provider === 'Prometheus' ? PROMETHEUS_CONFIG : OTEL_CONFIG;

  return (await axios.put(`opni-api/NodeConfiguration/node_config/${ id }`, { spec })).data;
}

const PROMETHEUS_CONFIG = {
  rules:      { discovery: { prometheusRules: {} } },
  prometheus: { deploymentStrategy: 'externalPromOperator' }
};

const OTEL_CONFIG = {
  rules:  { discovery: { prometheusRules: {} } },
  otel:  { hostMetrics: false }
};

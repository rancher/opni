import axios from 'axios';
import { isEmpty } from 'lodash';
import { Cluster } from '../../models/Cluster';
import { Slo, SlosResponse, SloStatusResponse, SloStatusStateResponse } from '../../models/Slo';
import { SloMetricsResponse } from '../../models/SloMetric';
import { SloService, SloServicesResponse } from '../../models/SloService';
import { getClusters } from './management';

export type Datasource = 'monitoring' | 'logging';

export async function getServices(clusterId: string): Promise<SloService[]> {
  const clustersRequest = getClusters(null);
  const data = (await axios.post<SloServicesResponse>(`opni-api/SLO/services`, { datasource: 'monitoring', clusterId }))?.data;
  const response = isEmpty(data) ? { items: [] } : data;
  const clusters = await clustersRequest;

  return response.items.map(item => new SloService(item, clusters, null) );
}

export async function getMetrics(clusterId: string, serviceId: string): Promise<any> {
  const response = (await axios.post<SloMetricsResponse>(`opni-api/SLO/metrics`, {
    datasource: 'monitoring', clusterId, serviceId
  }))?.data || { groupNameToMetrics: {} };

  return response;
}

export async function getEvents(clusterId: string, serviceId: string, metricId: string): Promise<any> {
  const response = (await axios.post<SloMetricsResponse>(`opni-api/SLO/events`, {
    datasource: 'monitoring', clusterId, serviceId, metricId
  }))?.data || { items: [] };

  return response.items;
}

export function createSLO(name: string, cluster: string, service: string, goodMetricName: string, totalMetricName: string, goodEvents: any[], totalEvents: any[], period: string, budgetingInterval: string, targetValue: number, tags: string[], attachedEndpoints: any) {
  const body = {
    slo: {
      name,
      datasource: 'monitoring',
      clusterId:  cluster,
      serviceId:  service,
      goodMetricName,
      totalMetricName,
      goodEvents,
      totalEvents,
      sloPeriod:  period,
      budgetingInterval,
      target:     { value: targetValue },
      labels:     tags.map(t => ({ name: t })),
      attachedEndpoints
    },
  };

  return axios.post(`opni-api/SLO/slos`, body);
}

export function updateSLO(id: string, name: string, cluster: string, service: string, goodMetricName: string, totalMetricName: string, goodEvents: any[], totalEvents: any[], period: string, budgetingInterval: string, targetValue: number, tags: string[], attachedEndpoints: any) {
  const body = {
    id,
    SLO: {
      name,
      datasource: 'monitoring',
      clusterId:  cluster,
      serviceId:  service,
      goodMetricName,
      totalMetricName,
      goodEvents,
      totalEvents,
      sloPeriod:  period,
      budgetingInterval,
      target:     { value: targetValue },
      labels:     tags.map(t => ({ name: t })),
      attachedEndpoints
    },
  };

  return axios.put(`opni-api/SLO/slos/${ id }/update`, body);
}

export async function previewSLO(name: string, cluster: string, service: string, goodMetricName: string, totalMetricName: string, goodEvents: any[], totalEvents: any[], period: string, budgetingInterval: string, targetValue: number, tags: string[]) {
  const body = {
    slo: {
      name,
      datasource: 'monitoring',
      clusterId:  cluster,
      serviceId:  service,
      goodMetricName,
      totalMetricName,
      goodEvents,
      totalEvents,
      sloPeriod:  period,
      budgetingInterval,
      target:     { value: targetValue },
      labels:     tags.map(t => ({ name: t }))
    },
  };

  return (await axios.post(`opni-api/SLO/slos/preview`, body)).data;
}

export function deleteSLO(id: string) {
  return axios.delete(`opni-api/SLO/slos/`, { data: { id } });
}

export function cloneSLOToClusters(id: string, clusterIds: string[]) {
  const body = {
    cloneId:  { id },
    clusters: clusterIds.map(id => ({ id }))
  };

  return axios.post(`opni-api/SLO/slos/${ id }/clone`, body);
}

export async function getSLO(id: string, vue: any) {
  // This doesn't work currently because the service doesn't support params being a part of the url and get requests can't have a body from browsers.
  // const response = (await axios.get<SloResponse>(`opni-api/SLO/slos/${ id }`))?.data;

  const slos = await getSLOs(vue);
  const slo = slos.find(s => s.id === id);

  if (!slo) {
    return false;
  }

  return slo;
}

export async function getSLOs(vue: any, clusters?: Cluster[]) {
  const response = (await axios.get<SlosResponse>('opni-api/SLO/slos'))?.data || { items: [] };

  return response?.items?.map(item => new Slo(item, vue, clusters)) || [];
}

export async function getSLOStatus(id: string): Promise<SloStatusStateResponse> {
  const result = (await axios.post <SloStatusResponse>(`opni-api/SLO/slos/${ id }/status`, { id })).data;

  return result?.state;
}

import axios from 'axios';
import { Reference } from '../../models/shared';

import {
  AlertCondition, AlertConditionList, AlertDetailChoicesRequest, AlertStatusResponse, Condition, ConditionReference, ListAlarmMessageRequest, ListAlertTypeDetails, ListMessageResponse, ListStatusResponse, SilenceRequest, TimelineRequest, TimelineResponse, UpdateAlertConditionRequest
} from '../../models/alerting/Condition';
import {
  AlertEndpoint, AlertEndpointList, Endpoint, UpdateAlertEndpointRequest
} from '../../models/alerting/Endpoint';
import { Cluster } from '../../models/Cluster';

export async function createAlertEndpoint(endpoint: AlertEndpoint) {
  await axios.post('opni-api/AlertEndpoints/configure', endpoint);
}

export async function updateAlertEndpoint(endpoint: UpdateAlertEndpointRequest) {
  await axios.put('opni-api/AlertEndpoints/configure', endpoint);
}

export async function getAlertEndpoints(vue: any): Promise<Endpoint[]> {
  const response = (await axios.get <AlertEndpointList>('opni-api/AlertEndpoints/list')).data;

  return (response.items || []).map(item => new Endpoint(item, vue));
}

export async function getAlertEndpoint(id: string, vue: any): Promise<Endpoint> {
  const response = (await axios.post<AlertEndpoint>(`opni-api/AlertEndpoints/list/${ id }`, { id })).data;

  return new Endpoint({ id: { id }, endpoint: response }, vue);
}

export async function testAlertEndpoint(request: Reference) {
  await axios.post<AlertEndpoint>(`opni-api/AlertNotifications/test`, request);
}

export function deleteEndpoint(id: string) {
  return axios.post(`opni-api/AlertEndpoints/delete`, { id: { id }, forceDelete: false });
}

export function createAlertCondition(alertCondition: AlertCondition) {
  return axios.post(`opni-api/AlertConditions/configure`, alertCondition);
}

export async function getAlertCondition(id: ConditionReference, vue: any): Promise<Condition> {
  return (await getAlertConditionsWithStatus(vue, { groupIds: [id.groupId] })).find(c => c.id === id.id) as Condition;
}

export async function getAlertConditionsWithStatus(vue: any, itemFilter?: { clusters?: string[], groupIds?: string[], alertTypes?: string[] }, clusters?: Cluster[]) {
  const response = (await axios.post<ListStatusResponse>('opni-api/AlertConditions/list/withStatus', { itemFilter })).data;

  return (Object.values(response.alertConditions) || []).map(conditionWithStatus => new Condition(conditionWithStatus, vue, clusters));
}

export async function cloneAlertCondition(condition: AlertCondition, clusterIds: string[]) {
  const body = {
    alertCondition: condition,
    toClusters:     clusterIds
  };

  (await axios.post<AlertConditionList>('opni-api/AlertConditions/clone', body));
}

export async function updateAlertCondition(condition: UpdateAlertConditionRequest): Promise<any> {
  return await axios.put('opni-api/AlertConditions/configure', condition);
}

export async function getAlertConditionChoices(request: AlertDetailChoicesRequest): Promise<ListAlertTypeDetails> {
  return (await axios.post<ListAlertTypeDetails>('opni-api/AlertConditions/choices', request)).data;
}

export function deleteAlertCondition(id: string) {
  return axios.delete(`opni-api/AlertConditions/configure`, { data: { id } });
}

export async function getAlertConditionStatus(id: string): Promise<AlertStatusResponse> {
  return (await axios.post(`opni-api/AlertConditions/status/${ id }`, { id })).data;
}

export function silenceAlertCondition(request: SilenceRequest) {
  return axios.post(`opni-api/AlertConditions/silences`, request);
}

export function deactivateSilenceAlertCondition(id: ConditionReference) {
  return axios.delete(`opni-api/AlertConditions/silences`, { data: id });
}

export async function getConditionTimeline(request: TimelineRequest): Promise<TimelineResponse> {
  return (await axios.post<TimelineResponse>(`opni-api/AlertConditions/timeline`, request)).data;
}

export async function getAlarmNotifications(request: ListAlarmMessageRequest): Promise<ListMessageResponse> {
  return (await axios.post<ListMessageResponse>(`opni-api/AlertNotifications/alarms/list`, request)).data;
}

export async function getAlertConditionGroups(): Promise<Reference[]> {
  return (await axios.get(`opni-api/AlertConditions/groups`)).data?.items || [];
}

export async function getAlertConditionGroupIds(): Promise<string[]> {
  return (await getAlertConditionGroups()).map(g => g.id);
}

export interface ResourceLimitSpec {
  // Storage resource limit for alerting volume
  storage: string;
  // CPU resource limit per replica
  cpu: string;
  // Memory resource limit per replica
  memory: string;
}
export interface ClusterConfiguration {
  // number of replicas for the opni-alerting (odd-number for HA)
  numReplicas: number;

  // Maximum time to wait for cluster
  // connections to settle before
  // evaluating notifications.
  clusterSettleTimeout: string;
  // Interval for gossip state syncs.
  // Setting this interval lower
  // (more frequent) will increase
  // convergence speeds across larger
  // clusters at the expense of
  // increased bandwidth usage.
  clusterPushPullInterval: string;
  // Interval between sending gossip
  // messages. By lowering this
  // value (more frequent) gossip
  // messages are propagated across
  // the cluster more quickly at the
  // expense of increased bandwidth.
  clusterGossipInterval: string;

  resourceLimits: ResourceLimitSpec;
}

export enum InstallState {
  InstallUnknown = 0, // eslint-disable-line no-unused-vars
  NotInstalled = 1, // eslint-disable-line no-unused-vars
  InstallUpdating = 2, // eslint-disable-line no-unused-vars
  Installed = 3, // eslint-disable-line no-unused-vars
  Uninstalling = 4, // eslint-disable-line no-unused-vars
}

export enum AlertType {
  System = 0,
  KubeState = 1,
  CpuSaturation = 2,
  MemorySaturation = 3,
  FsSaturation = 4,
  DownstreamCapability = 5,
  PrometheusQuery = 9,
  MonitoringBackend = 10,
}

export enum AlertTypeToString {
  System = 'System',
  KubeState = 'KubeState',
  CpuSaturation = 'CpuSaturation',
  MemorySaturation = 'MemorySaturation',
  FsSaturation = 'FsSaturation',
  DownstreamCapability = 'DownstreamCapability',
  PrometheusQuery = 'PrometheusQuery',
  MonitoringBackend = 'MonitoringBackend',
}

export const AlertTypeStringToNumber = {
  System:               0,
  KubeState:            1,
  CpuSaturation:        2,
  MemorySaturation:     3,
  FsSaturation:         4,
  DownstreamCapability: 5,
  PrometheusQuery:      9,
  MonitoringBackend:    10,
};

export interface InstallStatus {
  state: InstallState;
  version: string;
  metadata: { [key: string]: string };
}

export async function getClusterConfiguration(): Promise<ClusterConfiguration> {
  return (await axios.get<ClusterConfiguration>(`opni-api/AlertingAdmin/configuration`)).data;
}
// Install/Uninstall the alerting cluster by setting enabled=true/false
export async function configureCluster(config: ClusterConfiguration) {
  try {
    await axios.post(`opni-api/AlertingAdmin/configure`, config);
  } catch (ex) {
    if (ex?.response?.data !== 'no changes to apply') {
      throw ex;
    }
  }
}
export async function getClusterStatus(): Promise<InstallStatus> {
  return (await axios.get<InstallStatus>(`opni-api/AlertingAdmin/status`)).data;
}

export function installCluster() {
  return axios.post(`opni-api/AlertingAdmin/install`);
}
export function uninstallCluster() {
  return axios.post(`opni-api/AlertingAdmin/uninstall`);
}

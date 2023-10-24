import axios from 'axios';

export interface CPUResource {
  request: string;
  limit: string;
}

export interface DataPersistence {
  enabled?: boolean;
  storageClass?: string;
}

export interface Toleration {
  key: string;
  operator: string;
  value: string;
  taintEffect: string;
}

export interface ComputeResourceQuantities {
  cpu: string;
  memory: string;
}

export interface ResourceRequirements {
  requests: ComputeResourceQuantities;
  limits: ComputeResourceQuantities;
}

export interface DataDetails {
  replicas?: string;
  diskSize: string;
  memoryLimit: string;
  cpuResources?: CPUResource;
  enableAntiAffinity ?: boolean;
  nodeSelector: {[key: string]: string};
  tolerations: Toleration[];
  persistence?: DataPersistence;
}

export interface DashboardsDetails {
    enabled?: boolean;
    replicas?: string;
    resources: ResourceRequirements;
}

export interface IngestDetails {
  replicas?: string;
  memoryLimit: string;
  cpuResources ?: CPUResource;
  enableAntiAffinity ?: boolean;
  nodeSelector: { [key: string]: string };
  tolerations: Toleration[];
}

export interface ControlplaneDetails {
  replicas?: string;
  nodeSelector: { [key: string]: string };
  tolerations: Toleration[];
  persistence?: DataPersistence;
}

export interface OpensearchClusterV2 {
  externalURL: string;
  dataNodes?: DataDetails;
  ingestNodes?: IngestDetails;
  controlplaneNodes?: ControlplaneDetails;
  dashboards?: DashboardsDetails;
  dataRetention?: string;
}

export interface ProxySettings {
  proxyHost: string;
  proxyPort?: number;
}

export interface S3Credentials {
  accessKey: string;
  secretKey: string;
}
export interface OpensearchS3Settings {
  endpoint: string;
  insecure: boolean;
  pathStyleAccess: boolean;
  credentials: S3Credentials;
  bucket?: string;
  folder: string;
  proxySettings: ProxySettings;
}

export interface UpgradeAvailableResponse {
  upgradePending: boolean;
}

export interface StorageClassResponse {
  storageClasses: string[];
}

export interface StatusResponse {
  status: string;
  details: string;
}

export enum Status {
  ClusterStatusPending = 1,
  ClusterStatusGreen,
  ClusterStatusYellow,
  ClusterStatusRed,
  ClusterStatusError
}

export interface SnapshotReference {
  name: string;
}

export interface SnapshotRetention {
  timeRetention: string;
  maxSnapshots: number;
}

export interface SnapshotStatus {
  ref: SnapshotReference;
  status: string;
  statusMessage: string;
  lastUpdated: string;
  recurring: boolean;
}

export interface SnapshotStatusList {
  statuses: SnapshotStatus[];
}

export interface Snapshot {
  ref: SnapshotReference;
  cronSchedule: string;
  retention: SnapshotRetention;
  additionalIndices: string;
}

export async function getOpensearchCluster(): Promise<OpensearchClusterV2> {
  return (await axios.get('opni-api/LoggingAdminV2/logging/cluster')).data;
}

export async function deleteOpensearchCluster() {
  return (await axios.delete('opni-api/LoggingAdminV2/logging/cluster')).data;
}

export async function createOrUpdateOpensearchCluster(options: OpensearchClusterV2) {
  return (await axios.put('opni-api/LoggingAdminV2/logging/cluster', options)).data;
}

export async function upgradeAvailable(): Promise<UpgradeAvailableResponse> {
  return (await axios.get('opni-api/LoggingAdminV2/logging/upgrade/available')).data;
}

export async function doUpgrade() {
  return (await axios.post('opni-api/LoggingAdminV2/logging/upgrade/do')).data;
}

export async function getStorageClasses(): Promise<string[]> {
  return (await axios.get('opni-api/LoggingAdminV2/logging/storageclasses')).data?.storageClasses || [];
}

export async function getOpensearchStatus(): Promise <StatusResponse> {
  return (await axios.get('opni-api/LoggingAdminV2/logging/status')).data;
}

export async function createOrUpdateSnapshot(input: Snapshot) {
  return await axios.put('opni-api/LoggingAdminV2/logging/snapshot', input);
}

export async function getRecurringSnapshot(input: SnapshotReference): Promise<Snapshot> {
  return (await axios.get(`opni-api/LoggingAdminV2/logging/snapshot/${ input.name }`)).data;
}

export async function deleteSnapshot(input: SnapshotReference) {
  return (await axios.delete(`opni-api/LoggingAdminV2/logging/snapshot/${ input.name }`)).data;
}

export async function listSnapshots(): Promise<SnapshotStatusList> {
  return (await axios.get(`opni-api/LoggingAdminV2/logging/snapshot`)).data?.statuses || [];
}

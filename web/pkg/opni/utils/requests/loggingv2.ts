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
  // eslint-disable-next-line no-unused-vars
  ClusterStatusPending = 1,
  // eslint-disable-next-line no-unused-vars
  ClusterStatusGreen,
  // eslint-disable-next-line no-unused-vars
  ClusterStatusYellow,
  // eslint-disable-next-line no-unused-vars
  ClusterStatusRed,
  // eslint-disable-next-line no-unused-vars
  ClusterStatusError
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

export async function GetOpensearchStatus(): Promise <StatusResponse> {
  return (await axios.get('opni-api/LoggingAdminV2/logging/status')).data;
}

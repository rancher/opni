import axios from 'axios';

interface ResourceRequirements {
  limits: { [key: string]: string };
  requests: { [key: string]: string };
}

interface Toleration {
  key: string;
  operator: string;
  value: string;
  taintEffect: string;
}

interface DataPersistence {
    Enabled?: boolean;
    StorageClass?: string;
}

interface CPUResource {
    Request: string; // k8s.io.apimachinery.pkg.api.resource.Quantity
    Limit: string; // k8s.io.apimachinery.pkg.api.resource.Quantity
}

interface OpensearchNodeDetails {
    Name: string;
    Replicas?: number;
    DiskSize: string; // k8s.io.apimachinery.pkg.api.resource.Quantity
    MemoryLimit: string; // k8s.io.apimachinery.pkg.api.resource.Quantity
    CPUResources?: CPUResource;
    EnableAntiAffinity?: boolean;
    NodeSelector: { [key: string]: string};
    Tolerations: Toleration[]; // repeated k8s.io.api.core.v1.Toleration
    Roles: string[];
    Persistence?: DataPersistence;
}

interface DashboardsDetails {
    Enabled?: boolean;
    Replicas?: Number;
    Resources?: ResourceRequirements; // k8s.io.api.core.v1.ResourceRequirements
}

interface UpgradeAvailableResponse {
    UpgradePending: boolean;
}

interface OpensearchCluster {
    ExternalURL: string;
    NodePools: OpensearchNodeDetails[];
    Dashboards?: DashboardsDetails;
    DataRetention: string;
}

export async function getStorageClasses(): Promise<string[]> {
  return (await axios.get('/opni-api/LoggingAdmin/logging/storageclasses')).data?.storageClasses || [];
}

export async function getLoggingCluster(): Promise<OpensearchCluster> {
  return (await axios.get('/opni-api/LoggingAdmin/logging/cluster')).data;
}

export async function deleteLoggingCluster(): Promise<any> {
  await axios.delete('/opni-api/LoggingAdmin/logging/cluster');
}

export async function upsertLoggingCluster(cluster: OpensearchCluster): Promise<any> {
  return await axios.put('/opni-api/LoggingAdmin/logging/cluster', cluster);
}

export async function isLoggingClusterUpgradeAvailable(): Promise<UpgradeAvailableResponse> {
  return (await axios.get('/opni-api/LoggingAdmin/logging/upgrade/available')).data;
}

export async function upgradeLoggingCluster() {
  await axios.post('/opni-api/LoggingAdmin/logging/upgrade/do');
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
export interface StatusResponse {
  status: Status;
  details: string;
}

export async function getLoggingStatus(): Promise<UpgradeAvailableResponse> {
  return (await axios.get < UpgradeAvailableResponse>('/opni-api/LoggingAdmin/logging/status')).data;
}

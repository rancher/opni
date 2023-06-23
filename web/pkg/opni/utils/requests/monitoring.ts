import axios from 'axios';

export enum DeploymentMode {
    AllInOne = 0, // eslint-disable-line no-unused-vars
    HighlyAvailable = 1 // eslint-disable-line no-unused-vars
}

export interface SSEConfig {
    type: string;
    kmsKeyID: string;
    kmsEncryptionContext: string;
}

export interface GrafanaConfig {
  enabled: boolean;
  hostname: string;
}

export interface HTTPConfig {
    idleConnTimeout: string;
    responseHeaderTimeout: string;
    insecureSkipVerify: boolean;
    tlsHandshakeTimeout: string;
    expectContinueTimeout: string;
    maxIdleConns: number;
    maxIdleConnsPerHost: number;
    maxConnsPerHost: number;
}

export interface S3StorageSpec {
    endpoint: string;
    region: string;
    bucketName: string;
    secretAccessKey: string;
    accessKeyID: string;
    insecure: boolean;
    signatureVersion: string;
    sse: SSEConfig;
    http: HTTPConfig;
}

export interface FilesystemStorageSpec {
    directory: string;
}

export enum StorageBackend {
  Filesystem = 0,
  S3 = 1,
  GCS = 2,
  Azure = 3,
  Swift = 4,
}

export interface StorageSpec {
    backend: StorageBackend;
    s3?: S3StorageSpec;
    filesystem?: FilesystemStorageSpec;
}

export interface ClusterConfiguration {
    mode: DeploymentMode;
    storage?: StorageSpec;
    grafana?: GrafanaConfig;
}

export enum InstallState {
    Unknown = 0,
    NotInstalled = 1,
    Updating = 2,
    Installed = 3,
    Uninstalling = 4,
}

export interface InstallStatus {
    state: InstallState;
    version: string;
    metadata: { [key: string]: string};
}

export async function getClusterConfig(): Promise<ClusterConfiguration> {
  return (await axios.get('opni-api/CortexOps/configuration')).data;
}

export async function configureCluster(config: ClusterConfiguration) {
  await axios.post('opni-api/CortexOps/configure', config);
}

export async function getClusterStatus(): Promise<InstallStatus> {
  return (await axios.get('opni-api/CortexOps/status')).data;
}

export async function uninstallCluster() {
  await axios.post('opni-api/CortexOps/uninstall');
}

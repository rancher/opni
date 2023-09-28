import axios from 'axios';
import { HandleClusterHealthStatusEvent, HandleClusterWatchEvent } from '@pkg/opni/store';
import { Empty } from '@bufbuild/protobuf';
import { NodeCapabilityStatus } from '../../models/Capability';
import { Management } from '../../api/opni';
import { WatchClustersRequest } from '../../generated/github.com/rancher/opni/pkg/apis/management/v1/management_pb';
import { TokensResponse, Token } from '../../models/Token';
import { CapabilityStatusResponse } from '../../models/Cluster';
import { MatchLabel, Role, RoleResponse, RolesResponse } from '../../models/Role';
import { RoleBinding, RoleBindingsResponse } from '../../models/RoleBinding';
import { GatewayConfig, ConfigDocument } from '../../models/Config';
import { LABEL_KEYS } from '../../models/shared';
import { base64Encode } from '../crypto';

export async function installCapabilityV2(capability: string, clusterId: string) {
  return (await axios.post(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capability }/install`)).data;
}

export async function uninstallCapabilityStatusV2(capability: string, clusterId: string) {
  return await axios.get(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capability }/uninstall/status`);
}

export interface CapabilityInstallerResponse {
  command: string;
}

export interface DashboardGlobalSettings {
  defaultImageRepository?: string;
  defaultTokenTtl?: string;
  defaultTokenLabels?: { [key: string]: string };
}
export interface DashboardSettings {
  global?: DashboardGlobalSettings;
  user?: { [key: string]: string };
}

export async function getTokens(vue: any) {
  const tokensResponse = (await axios.get<TokensResponse>(`opni-api/Management/tokens`)).data.items;

  return tokensResponse.map(tokenResponse => new Token(tokenResponse, vue));
}

export async function getDefaultImageRepository() {
  return (await axios.get<DashboardSettings>(`opni-api/Management/dashboard/settings`)).data.global?.defaultImageRepository;
}

export async function getCapabilities(vue: any) {
  const capabilitiesResponse = (await axios.get<any>(`opni-api/Management/capabilities`)).data.items;

  return capabilitiesResponse;
}

export function uninstallCapability(clusterId: string, capability: string, deleteStoredData: boolean, vue: any) {
  const initialDelay = deleteStoredData ? { initialDelay: '1m' } : {};

  return axios.post<any>(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capability }/uninstall`, { options: { ...initialDelay, deleteStoredData } });
}

export async function cancelCapabilityUninstall(clusterId: string, capabilityName: string) {
  await axios.post(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capabilityName }/uninstall/cancel`, {
    name:    capabilityName,
    cluster: { id: clusterId }
  });
}

export async function uninstallCapabilityStatus(clusterId: string, capability: string, vue: any) {
  return (await axios.get<CapabilityStatusResponse>(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capability }/uninstall/status`)).data;
}

export async function getCapabilityInstaller(capability: string, token: string, pin: string) {
  return (await axios.post<CapabilityInstallerResponse>(`opni-api/Management/capabilities/${ capability }/installer`, {
    token,
    pin,
  })).data.command;
}

export async function getCapabilityStatus(clusterId: string, capability: string, vue: any): Promise<NodeCapabilityStatus> {
  return (await axios.get<NodeCapabilityStatus>(`opni-api/Management/clusters/${ clusterId }/capabilities/${ capability }/status`)).data;
}

export async function createToken(ttlInSeconds: string, name: string | null, capabilities: any[]) {
  const labels = name ? { labels: { [LABEL_KEYS.NAME]: name } } : { labels: {} };

  const raw = (await axios.post<any>(`opni-api/Management/tokens`, {
    ttl: ttlInSeconds,
    ...labels,
    capabilities,
  })).data;

  return new Token(raw, null);
}

export function deleteToken(id: string): Promise<undefined> {
  return axios.delete(`opni-api/Management/tokens/${ id }`);
}

export interface CertResponse {
  issuer: string;
  subject: string;
  isCA: boolean;
  notBefore: string;
  notAfter: string;
  fingerprint: string;
}

export interface CertsResponse {
  chain: CertResponse[];
}

export async function getCerts(): Promise<CertResponse[]> {
  return (await axios.get<CertsResponse>(`opni-api/Management/certs`)).data.chain;
}

export async function getClusterFingerprint() {
  const certs = await getCerts();

  return certs.length > 0 ? certs[certs.length - 1].fingerprint : {};
}

export async function updateCluster(id: string, name: string, labels: { [key: string]: string }) {
  labels = { ...labels, [LABEL_KEYS.NAME]: name };
  if (name === '') {
    delete labels[LABEL_KEYS.NAME];
  }
  (await axios.put<any>(`opni-api/Management/clusters/${ id }`, {
    cluster: { id },
    labels
  }));
}

export function watchClusters(store: any) {
  const request = new WatchClustersRequest();

  Management.service.WatchClusters(request, e => store.dispatch(HandleClusterWatchEvent, e));
  Management.service.WatchClusterHealthStatus(new Empty(), e => store.dispatch(HandleClusterHealthStatusEvent, e));
}

export function deleteCluster(id: string): Promise<undefined> {
  return axios.delete(`opni-api/Management/clusters/${ id }`);
}

export async function getRoles(vue: any): Promise<Role[]> {
  const rolesList = (await axios.get<RolesResponse>(`opni-api/Management/rbac/backend/metrics/roles`)).data.items;

  return rolesList.map(ref => (
    new Role({id: ref.id, permissions: []}, vue)
  ));
}

export async function getRole(id: string, vue: any): Promise<Role> {
  const roleResponse = (await axios.get<RoleResponse>(`opni-api/Management/rbac/backend/metrics/roles/${ id }`)).data;

  return new Role(roleResponse, vue)
}

export function deleteRole(id: string): Promise<undefined> {
  return axios.delete(`opni-api/Management/rbac/backend/metrics/roles/${ id }`);
}

export async function createRole(name: string, clusterIDs: string[], matchLabels: MatchLabel) {
  const capability: string = "metrics"
  const permissions = [{
    ids: clusterIDs,
    type: "cluster",
    matchLabels: matchLabels,
    verbs: [{verb: "GET"}],
  }];
  const role = {
    id: name,
    permissions: permissions,
  };
  (await axios.post<any>(`opni-api/Management/rbac/backend/metrics/roles`, {
    capability: { name: capability }, role
  }));
}

export async function getRoleBindings(vue: any): Promise<RoleBinding[]> {
  const roleBindingsResponse = (await axios.get<RoleBindingsResponse>(`opni-api/Management/rolebindings`)).data.items;

  return roleBindingsResponse.map(roleBindingResponse => new RoleBinding(roleBindingResponse, vue));
}

export function deleteRoleBinding(id: string): Promise<undefined> {
  return axios.delete(`opni-api/Management/rolebindings/${ id }`);
}

export async function createRoleBinding(name: string, roleName: string, subjects: string[]) {
  const metadata = {
    capability: "metrics",
  };
  (await axios.post<any>(`opni-api/Management/rolebindings`, {
    id: name, roleId: roleName, subjects, metadata
  }));
}

export async function getGatewayConfig(vue: any): Promise<ConfigDocument[]> {
  const config = (await axios.get<GatewayConfig>(`opni-api/Management/config`)).data;

  return config.documents.map(configDocument => new ConfigDocument(configDocument, vue));
}

export function updateGatewayConfig(jsonDocuments: string[]): Promise<undefined> {
  const documents = [];

  for (const jsonDocument of jsonDocuments) {
    documents.push({ json: base64Encode(jsonDocument) });
  }

  return axios.put(`opni-api/Management/config`, { documents });
}

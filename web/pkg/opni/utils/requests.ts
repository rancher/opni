import axios from 'axios';
import { ClusterStats, ClusterStatsList } from '../models/Cluster';
import { getClusterFingerprint } from './requests/management';

export interface DashboardGlobalSettings {
  defaultImageRepository?: string;
  defaultTokenTtl?: string;
  defaultTokenLabels?: { [key: string]: string };
}
export interface DashboardSettings {
  global?: DashboardGlobalSettings;
  user?: { [key: string]: string};
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

export async function createAgent(tokenId: string) {
  const fingerprint = await getClusterFingerprint();

  (await axios.post<any>(`opni-test/agents`, { token: tokenId, pins: [fingerprint] }));
}

export async function getClusterStats(vue: any): Promise<ClusterStats[]> {
  const clustersResponse = (await axios.get<ClusterStatsList>(`opni-api/CortexAdmin/all_user_stats`)).data.items;

  return clustersResponse;
}

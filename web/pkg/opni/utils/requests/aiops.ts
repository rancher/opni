import axios from 'axios';

export interface PretrainedModel {
    httpSource?: string;
    imageSource?: string;
    replicas?: number;
}

export interface GPUSettings {
    runtimeClass?: string;
}

export interface S3Settings {
  endpoint?: string,
  accessKey?: string,
  secretKey?: string,
  nulogBucket?: string,
  drainBucket?: string,
}

export interface AISettings {
    gpuSettings?: GPUSettings;
    s3Settings?: S3Settings;
    drainReplicas?: number;
    controlplane?: PretrainedModel;
    rancher?: PretrainedModel;
    longhorn?: PretrainedModel;
}

export interface UpgradeAvailableResponse {
    UpgradePending: boolean;
}

export interface RuntimeClassResponse {
    RuntimeClasses: string[];
}

export async function getAISettings(): Promise<AISettings | null> {
  try {
    return (await axios.get<AISettings>('opni-api/AIAdmin/ai/settings')).data;
  } catch (ex) {
    return null;
  }
}

export async function deleteAISettings() {
  await axios.delete('opni-api/AIAdmin/ai/settings');
}

export async function updateAISettings(settings: AISettings) {
  await axios.put<AISettings>('opni-api/AIAdmin/ai/settings', settings);
}

export async function isUpgradeAvailable(): Promise<boolean> {
  return (await axios.get<UpgradeAvailableResponse>('opni-api/AIAdmin/ai/upgrade')).data.UpgradePending;
}

export async function upgrade() {
  await axios.post('opni-api/AIAdmin/ai/upgrade');
}

export async function getRuntimeClasses(): Promise<RuntimeClassResponse> {
  const r = (await axios.get<RuntimeClassResponse>('opni-api/AIAdmin/ai/runtimeclasses')).data;

  return r.RuntimeClasses ? r : { RuntimeClasses: [] };
}

import axios from 'axios';
import { isEmpty } from 'lodash';
import { Deployment, DeploymentResponse } from '../../models/Deployment';

export async function getDeployments(clusterId: string): Promise<Deployment[]> {
  const deployments = (await axios.get<{ items: DeploymentResponse[] }>(`opni-api/ModelTraining/workload_aggregation/${ clusterId }`)).data;

  return (deployments?.items || []).map(d => new Deployment(d, null));
}

export interface ModelStatus {
  status: 'training' | 'completed' | 'training failed' | 'no model trained';
  statistics: {
    timeElapsed:string;
    percentageCompleted:string;
    remainingTime:string;
    currentEpoch:string;
    stage: 'train' | 'fetching data';
  }
}

export async function getModelStatus(): Promise<ModelStatus> {
  return await (await axios.get <ModelStatus>(`opni-api/ModelTraining/model/status`)).data;
}

export interface ModelTrainingParameters {
  clusterId: string;
  namespace: string;
  deployment: string;
}

export interface ModelTrainingParametersList {
  items: ModelTrainingParameters[];
}

export async function trainModel(parameters: ModelTrainingParameters[]) {
  await axios.post(`opni-api/ModelTraining/model/train`, { items: parameters });
}

export async function getModelTrainingParameters(): Promise<ModelTrainingParametersList> {
  const response = (await axios.get<ModelTrainingParametersList>(`opni-api/ModelTraining/model/training_parameters`)).data;

  return isEmpty(response) ? { items: [] } : response;
}

export interface GPUInfo {
  name: string;
  capacity: string;
  allocatable: string;
}

export interface GPUInfoList {
  items: GPUInfo[];
}

export async function hasGpu(): Promise<Boolean> {
  const gpuList = (await axios.get<GPUInfoList>(`opni-api/ModelTraining/gpu_info`)).data;
  const gpus = gpuList.items || [];

  return gpus.some(gpu => Number.parseInt(gpu.allocatable) > 0);
}

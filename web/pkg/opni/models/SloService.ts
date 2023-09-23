import { Resource } from './Resource';
import { Cluster } from './Cluster';

export interface SloServiceResponse {
  serviceId: string;
  clusterId: string;
}

export interface SloServicesResponse {
    items: SloServiceResponse[];
}

export class SloService extends Resource {
    private base: SloServiceResponse;

    constructor(base: SloServiceResponse, vue: any) {
      super(vue);
      this.base = base;
    }

    get id(): string {
      return this.base.serviceId;
    }

    get clusterId(): string {
      return this.base.clusterId;
    }

    get cluster(): Cluster {
      return this.vue.store.getters['opni/clusters'].find((cluster: Cluster) => cluster.id === this.clusterId) as Cluster;
    }
}

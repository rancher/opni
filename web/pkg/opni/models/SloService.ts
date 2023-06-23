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
    private clusters: Cluster[];

    constructor(base: SloServiceResponse, clusters: Cluster[], vue: any) {
      super(vue);
      this.base = base;
      this.clusters = clusters;
    }

    get id(): string {
      return this.base.serviceId;
    }

    get clusterId(): string {
      return this.base.clusterId;
    }

    get cluster(): Cluster {
      return this.clusters.find(cluster => cluster.id === this.clusterId) as Cluster;
    }
}

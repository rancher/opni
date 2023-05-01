import { Resource } from './Resource';

export interface SloMetricResponse {
    name: string;
    description: string;
    datasource: string;
    clusterId: string;
    serviceId: string;
    metricId: string;
}

export interface SloMetricsResponse {
    items: SloMetricResponse[];
}

export class SloMetric extends Resource {
    private base: SloMetricResponse;

    constructor(base: SloMetricResponse, vue: any) {
      super(vue);
      this.base = base;
    }

    get name(): string {
      return this.base.name;
    }

    get description(): string {
      return this.base.description;
    }

    get datasource(): string {
      return this.base.datasource;
    }

    get clusterId(): string {
      return this.base.clusterId;
    }

    get serviceId(): string {
      return this.base.serviceId;
    }

    get metricId(): string {
      return this.base.metricId;
    }
}

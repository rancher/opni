import { Resource } from './Resource';

export interface ConfigDocumentWithSchema {
  json: string;
  yaml: string;
  schema: string;
}

export interface GatewayConfig {
  documents: ConfigDocumentWithSchema[];
}

export class ConfigDocument extends Resource {
  private config: ConfigDocumentWithSchema;

  constructor(config: ConfigDocumentWithSchema, vue: any) {
    super(vue);
    this.config = config;
  }

  get schema(): string {
    return this.config.schema;
  }

  get yaml(): string {
    return Buffer.from(this.config.yaml, 'base64').toString();
  }

  get json(): any {
    return JSON.parse(Buffer.from(this.config.json, 'base64').toString());
  }
}

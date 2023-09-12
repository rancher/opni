import { capitalize } from 'lodash';
import { Resource } from '../Resource';
import { deleteEndpoint } from '../../utils/requests/alerts';
import { Reference } from '../shared';

export interface SlackEndpoint {
    webhookUrl: string;
    channel: string;
}

export interface EmailEndpoint {
    to: string;
    smtpFrom: string;
    smtpSmartHost: string;
    smtpAuthUsername: string;
    smtpAuthIdentity: string;
    smtpAuthPassword: string;
    smtpRequireTLS: boolean;
}

export interface PagerDutyEndpoint {
  integrationKey: string;
}

export interface BasicAuth {
  username: string;
  password: string;
  password_file: string; // eslint-disable-line camelcase
}

export interface Authorization {
  type: string;
  credentialsFile: string;
  credentials: string;
}

export interface TLSConfig {
  caFile: string;
  certFile: string;
  keyFile: string;
  serverName: string;
  insecureSkipVerify: boolean;
  minVersion: string;
  maxVersion: string;
}

export interface OAuth2 {
  clientId: string;
  clientSecret: string;
  clientSecretFile: string;
  scopes: string[];
  tokenUrl: string;
  proxyUrl: string;
  tlsConfig: TLSConfig;
  endpointParams: { [key: string]: string};
}

export interface HTTPConfig {
  basicAuth: BasicAuth;
  authorization: Authorization;
  oauth2: OAuth2;
  enabled_http2: boolean; // eslint-disable-line camelcase
  proxy_url: string; // eslint-disable-line camelcase
  follow_redirects: boolean; // eslint-disable-line camelcase
  tls_config: TLSConfig; // eslint-disable-line camelcase
}

export interface WebhookEndpoint {
  url: string;
  httpConfig: HTTPConfig;
  maxAlerts: number;
}
export interface AlertEndpoint {
    name: string;
    description: string;
    slack?: SlackEndpoint;
    email?: EmailEndpoint;
    pagerDuty?: PagerDutyEndpoint;
    webhook?: WebhookEndpoint;
}

export interface AlertEndpointWithId {
    endpoint: AlertEndpoint;
    id: Reference;
}

export interface AlertEndpointList {
    items: AlertEndpointWithId[];
}

export interface UpdateAlertEndpointRequest {
    forceUpdate: boolean;
    id: Reference;
    updateAlert: AlertEndpoint;
}

export interface TestAlertEndpointRequest {
  endpoint: AlertEndpoint;
}

export class Endpoint extends Resource {
    private base: AlertEndpointWithId;

    constructor(base: AlertEndpointWithId, vue: any) {
      super(vue);
      this.base = base;
    }

    get nameDisplay() {
      return this.base.endpoint.name;
    }

    get description() {
      return this.base.endpoint.description;
    }

    get id() {
      return this.base.id.id;
    }

    get type() {
      if (this.base.endpoint.email) {
        return 'email';
      }

      if (this.base.endpoint.slack) {
        return 'slack';
      }

      if (this.base.endpoint.pagerDuty) {
        return 'pagerDuty';
      }

      if (this.base.endpoint.webhook) {
        return 'webhook';
      }

      return 'unknown';
    }

    get typeDisplay() {
      return capitalize(this.type);
    }

    get endpoint() {
      return this.base.endpoint[this.type as keyof AlertEndpoint];
    }

    get availableActions(): any[] {
      return [
        {
          action:    'edit',
          altAction: 'edit',
          label:     'Edit',
          icon:      'icon icon-edit',
          enabled:   true,
        },
        {
          action:     'promptRemove',
          altAction:  'delete',
          label:      'Delete',
          icon:       'icon icon-trash',
          bulkable:   true,
          enabled:    true,
          bulkAction: 'promptRemove',
          weight:     -10, // Delete always goes last
        }
      ];
    }

    edit() {
      this.vue.$router.replace({
        name:   'endpoint',
        params: { id: this.id }
      });
    }

    async remove() {
      const result = await deleteEndpoint(this.id);

      if (result.data?.items?.length > 0) {
        throw new Error(`${ this.nameDisplay } is currently being used by 1 or more conditions and cannot be deleted.`);
      }

      super.remove();
    }
}

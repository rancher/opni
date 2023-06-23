import day from 'dayjs';
import { deleteToken } from '../utils/requests/management';
import { Labels, LABEL_KEYS } from './shared';
import { Resource } from './Resource';

export interface TokenCapability{
  type: string;
  reference: {
    id: string;
  };
}

export interface TokenMetadata {
  leaseID: string;
  ttl: string;
  usageCount: number;
  labels: Labels;
  capabilities: TokenCapability[];
}

export interface TokenResponse {
  tokenID: string;
  secret: string;
  metadata: TokenMetadata;
}

export interface TokensResponse {
  items: any[];
}

export class Token extends Resource {
    private base: TokenResponse;
    private now: day.Dayjs;

    constructor(base: TokenResponse, vue: any) {
      super(vue);
      this.base = base;
      this.now = day();
    }

    get type(): string {
      return 'token';
    }

    get id(): string {
      return `${ this.base.tokenID }.${ this.base.secret }`;
    }

    get name(): string {
      return this.base.metadata.labels[LABEL_KEYS.NAME];
    }

    get nameDisplay(): string {
      return this.name || this.id;
    }

    get nameDisplayShort(): string {
      return this.name || this.base.tokenID;
    }

    get expirationDate(): string {
      return this.now.add(Number.parseInt(this.ttl), 's').format();
    }

    get secret(): string {
      return this.base.secret;
    }

    get ttl(): string {
      return this.base.metadata.ttl;
    }

    get used(): number {
      return this.base.metadata.usageCount;
    }

    get usedDisplay(): string {
      return this.used === 1 ? 'Used 1 time' : `Used ${ this.used } times`;
    }

    get labels(): string[] {
      return Object.entries(this.base.metadata.labels)
        .filter(([key]) => !Object.values(LABEL_KEYS).includes(key))
        .map(([key, value]) => `${ key }=${ value }`);
    }

    get capabilities(): string[] {
      return this.base.metadata.capabilities.map((capability) => {
        if (capability.type === 'join_existing_cluster' && capability.reference.id) {
          const clusterId = capability.reference.id.slice(0, 8);

          return `join: ${ clusterId }`;
        }

        return '';
      }).filter(capability => capability !== '');
    }

    get availableActions(): any[] {
      return [
        {
          action:   'copy',
          label:    'Copy Token',
          icon:     'icon icon-copy',
          bulkable: false,
          enabled:  true,
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

    async remove() {
      await deleteToken(this.base.tokenID);
      super.remove();
    }
}

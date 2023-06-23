<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { cloneDeep } from 'lodash';
import Backend from '../Backend';
import CapabilityTable from '../CapabilityTable';
import { getMetricCapabilities } from '../../utils/requests/capability';
import { getClusterStats } from '../../utils/requests';
import {
  StorageBackend, InstallState, DeploymentMode, configureCluster, uninstallCluster, getClusterStatus, getClusterConfig
} from '../../utils/requests/monitoring';
import Grafana from './Grafana';
import Storage, { SECONDS_IN_DAY } from './Storage';

export async function isEnabled() {
  const status = (await getClusterStatus()).state;

  return status !== InstallState.NotInstalled;
}

export default {
  components: {
    Backend,
    LabeledSelect,
    Grafana,
    CapabilityTable,
    Storage,
    Tab,
    Tabbed
  },

  async fetch() {
    try {
      const config = await getClusterConfig();

      config.storage = config.storage || { backend: StorageBackend.S3 };
      const backendField = StorageBackend[config.storage.backend].toLowerCase();
      const clone = cloneDeep(this.config);

      this.$set(this.config, 'mode', config.mode || DeploymentMode.HighlyAvailable);
      this.$set(this, 'config', { ...clone, ...config });
      this.$set(this.config, 'storage', { ...clone.storage, ...config.storage });
      this.$set(this.config.storage, backendField, { ...clone.storage[backendField], ...config.storage[backendField] });
      this.$set(this.config.storage, 'backend', config.storage.backend);
      this.$set(this.config, 'grafana', config.grafana || { enabled: true });
    } catch (ex) { }
  },

  data() {
    return {
      modes:                      [
        {
          label: 'Standalone',
          value: DeploymentMode.AllInOne
        },
        {
          label: 'Highly Available',
          value: DeploymentMode.HighlyAvailable
        },
      ],
      loading:                    false,
      dashboardEnabled:           false,
      capabilities:               [],
      status:           InstallState.NotInstalled,
      config:           {
        mode:    DeploymentMode.HighlyAvailable,
        storage:       {
          backend:         StorageBackend.S3,
          retentionPeriod: `${ 30 * SECONDS_IN_DAY }s`,
          filesystem:       { directory: '' },
          s3:               {
            endpoint:         '',
            region:           'us-east-1',
            bucketName:       '',
            secretAccessKey:  '',
            accessKeyID:      '',
            insecure:         false,
            signatureVersion: 'v4',
            sse:              {
              type:                 '',
              kmsKeyID:             '',
              kmsEncryptionContext: '',
            },
            http: {
              idleConnTimeout:       '90s',
              responseHeaderTimeout: '120s',
              insecureSkipVerify:    false,
              tlsHandshakeTimeout:   '10s',
              expectContinueTimeout: '10s',
              maxIdleConns:          100,
              maxIdleConnsPerHost:   0,
              maxConnsPerHost:       100,
            },
          },
        },
        grafana: { enabled: true },
      },
    };
  },

  methods: {
    async updateStatus(capabilities = []) {
      try {
        const stats = await getClusterStats(this);

        capabilities.forEach(c => c.updateStats(stats));
      } catch (ex) {}
    },

    async loadCapabilities(parent) {
      this.capabilities = await getMetricCapabilities(parent);

      return this.capabilities;
    },

    headerProvider(headers) {
      const newHeaders = [...headers];

      newHeaders.push(...[
        {
          name:      'numSeries',
          labelKey:  'opni.tableHeaders.numSeries',
          sort:      ['numSeries'],
          value:     'numSeries',
          formatter: 'Number'
        },
        {
          name:          'sampleRate',
          labelKey:      'opni.tableHeaders.sampleRate',
          sort:          ['sampleRate'],
          value:         'sampleRate',
          formatter:     'Number',
          formatterOpts: { suffix: '/s' }
        },
        {
          name:          'provider',
          labelKey:      'opni.tableHeaders.provider',
          sort:          ['provider'],
          value:         'provider',
          formatter:     'TextWithClass',
          formatterOpts: {
            getClass(_, value) {
              return value === '—' ? 'text-muted' : '';
            }
          }
        },
        {
          name:          'isLocal',
          labelKey:      'opni.tableHeaders.local',
          sort:          ['isLocal'],
          value:         'localIcon',
          formatter:     'Icon',
          width:     20
        },
      ]);

      return newHeaders;
    },

    async disable() {
      await uninstallCluster();
      this.$set(this.config.storage.s3, 'secretAccessKey', '');
    },

    async save() {
      if (this.config.storage.backend === StorageBackend.S3) {
        if (this.config.storage.s3.endpoint === '') {
          throw new Error('Endpoint is required');
        }

        if (this.config.storage.s3.bucketName === '') {
          throw new Error('Bucket Name is required');
        }

        if (this.config.storage.s3.secretAccessKey === '') {
          throw new Error('Secret Access Key is required');
        }
      }

      if (this.config.grafana.enabled) {
        // check if hostname is set and not empty
        if (!this.config.grafana.hostname || this.config.grafana.hostname === '') {
          throw new Error('Grafana hostname is required');
        }
      }

      const copy = cloneDeep(this.config);

      if (this.config.storage.backend !== StorageBackend.Filesystem) {
        delete copy.storage.filesystem;
      }
      if (this.config.storage.backend !== StorageBackend.S3) {
        delete copy.storage.s3;
      }
      if (this.config.storage.backend !== StorageBackend.GCS) {
        delete copy.storage.gcs;
      }
      if (this.config.storage.backend !== StorageBackend.Azure) {
        delete copy.storage.azure;
      }
      if (this.config.storage.backend !== StorageBackend.Swift) {
        delete copy.storage.swift;
      }

      await configureCluster(copy);
    },

    bannerMessage(status) {
      switch (status) {
      case InstallState.Updating:
        return `Monitoring is currently updating on the cluster. You can't make changes right now.`;
      case InstallState.Uninstalling:
        return `Monitoring is currently uninstalling from the cluster. You can't make changes right now.`;
      case InstallState.Installed:
        return `Monitoring is currently installed on the cluster.`;
      default:
        return `Monitoring is currently in an unknown state on the cluster. You can't make changes right now.`;
      }
    },

    bannerState(status) {
      switch (status) {
      case InstallState.Updating:
      case InstallState.Uninstalling:
        return 'warning';
      case InstallState.Installed:
        return `success`;
      default:
        return `error`;
      }
    },

    async isEnabled() {
      return await isEnabled();
    },

    async isUpgradeAvailable() {
      return await false;
    },

    async getStatus() {
      try {
        const status = (await getClusterStatus()).state;

        if (status === InstallState.NotInstalled) {
          return null;
        }

        return {
          state:   this.bannerState(status),
          message: this.bannerMessage(status)
        };
      } catch (ex) {
        return null;
      }
    },
  },
};
</script>
<template>
  <Backend
    title="Monitoring"
    :is-enabled="isEnabled"
    :disable="disable"
    :is-upgrade-available="isUpgradeAvailable"
    :get-status="getStatus"
    :save="save"
  >
    <template #editing>
      <div class="row mb-20">
        <div class="col span-12">
          <LabeledSelect v-model="config.mode" :options="modes" label="Mode" />
        </div>
      </div>
      <Tabbed :side-tabs="true">
        <Tab :weight="4" name="storage" label="Storage">
          <Storage v-model="config" />
        </Tab>
        <Tab :weight="3" name="grafana" label="Grafana">
          <Grafana v-model="config.grafana" :status="status" />
        </Tab>
      </Tabbed>
    </template>
    <template #details>
      <CapabilityTable :capability-provider="loadCapabilities" :header-provider="headerProvider" :update-status-provider="updateStatus" />
    </template>
  </Backend>
</template>

<style lang="scss" scoped>
::v-deep {
  .tab-container {
    position: relative;
  }
}
</style>

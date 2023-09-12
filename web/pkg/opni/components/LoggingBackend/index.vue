<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import { cloneDeep } from 'lodash';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import {
  getOpensearchCluster, GetOpensearchStatus, createOrUpdateOpensearchCluster, doUpgrade, upgradeAvailable, Status, deleteOpensearchCluster
} from '@pkg/opni/utils/requests/loggingv2';
import Backend from '../Backend';
import CapabilityTable from '../CapabilityTable';
import DataPods from './DataPods';
import IngestPods from './IngestPods';
import ControlplanePods from './ControlplanePods';
import Dashboard from './Dashboard';
import { getStorageClassOptions } from './Storage';

export default {
  components: {
    Backend,
    CapabilityTable,
    IngestPods,
    DataPods,
    ControlplanePods,
    Dashboard,
    LabeledInput,
    Tabbed,
    Tab
  },

  async fetch() {
    await this.load();
  },

  data() {
    return {
      dashboardEnabled: false,
      enabled:          false,
      loading:          true,
      config:           {
        dataRetention: '7d',

        dataNodes: {
          replicas:     '1',
          diskSize:     '35Gi',
          memoryLimit:  '2048Mi',
          cpuResources: {
            request: '',
            limit:   ''
          },
          enableAntiAffinity: false,
          nodeSelector:       {},
          tolerations:        [],
          persistence:        {
            enabled:      false,
            storageClass: undefined
          }
        },

        ingestNodes: {
          enabled:      false,
          replicas:     '1',
          memoryLimit:  '1024Mi',
          cpuResources: {
            request: '',
            limit:   ''
          },
          enableAntiAffinity: false,
          nodeSelector:       {},
          tolerations:        [],
        },

        controlplaneNodes: {
          enabled:      false,
          replicas:     '1',
          nodeSelector: {},
          tolerations:  [],
          persistence:  {
            enabled:      false,
            storageClass: undefined
          },
        },

        dashboards: {
          enabled: true, replicas: '1', resources: { limits: { memory: '1024Mi' }, requests: { cpu: '3m' } }
        },

      },
      storageClassOptions: []
    };
  },

  methods: {
    enable() {
      this.$set(this, 'enabled', true);
    },

    async disable() {
      await deleteOpensearchCluster();
    },

    upgrade() {
      doUpgrade();
      this.$set(this, 'upgradeable', false);
    },

    async save() {
      if (this.config.dataRetention === '') {
        throw new Error('Data Retention is required');
      }

      if (!(/\d+(d|m)/g).test(this.config.dataRetention)) {
        throw new Error('Data Retention must be of the form <integer><time unit> i.e. 7d, 30d, 6m');
      }
      const modifiedConfig = cloneDeep(this.config);

      if (modifiedConfig.ingestNodes?.enabled) {
        delete modifiedConfig.ingestNodes.enabled;
      } else if (modifiedConfig.ingestNodes && !modifiedConfig.ingestNodes.enabled) {
        delete modifiedConfig.ingestNodes;
      }

      if (modifiedConfig.controlplaneNodes?.enabled) {
        delete modifiedConfig.controlplaneNodes.enabled;
      } else if (modifiedConfig.controlplaneNodes && !modifiedConfig.controlplaneNodes.enabled) {
        delete modifiedConfig.controlplaneNodes;
      }

      if (!modifiedConfig.dashboards?.enabled) {
        modifiedConfig.dashboards = { enabled: false };
      }

      await createOrUpdateOpensearchCluster(modifiedConfig);
    },

    async isEnabled() {
      const cluster = await getOpensearchCluster();

      return cluster && cluster.dataNodes;
    },

    async isUpgradeAvailable() {
      const upgradeable = await upgradeAvailable();

      return upgradeable.UpgradePending;
    },

    bannerState(status) {
      if (!status) {
        return '';
      }

      switch (status) {
      case Status.ClusterStatusGreen:
        return 'success';
      case Status.ClusterStatusPending:
      case Status.ClusterStatusYellow:
        return 'warning';
      default:
        return 'error';
      }
    },

    bannerMessage(status) {
      return status ? status.details : '';
    },

    async getStatus() {
      try {
        const status = await GetOpensearchStatus();

        return {
          state:   this.bannerState(status.status),
          message: this.bannerMessage(status)
        };
      } catch (ex) {
        return null;
      }
    },

    async load() {
      try {
        const cluster = await getOpensearchCluster();

        if (!cluster.dataNodes) {
          delete cluster.dataNodes;
        }

        const config = { ...this.config, ...cluster };

        config.ingestNodes.enabled = !!cluster.ingestNodes;
        config.controlplaneNodes.enabled = !!cluster.controlplaneNodes;

        this.$set(this, 'config', config);
        this.$set(this, 'storageClassOptions', await getStorageClassOptions());
      } catch (ex) {}
    }
  }
};
</script>
<template>
  <Backend
    title="Logging"
    :is-enabled="isEnabled"
    :disable="disable"
    :is-upgrade-available="isUpgradeAvailable"
    :get-status="getStatus"
    :save="save"
  >
    <template #editing>
      <div class="row mb-20">
        <div class="col span-12">
          <LabeledInput
            v-model="config.dataRetention"
            label="Data Retention"
            placeholder="e.g. 7d, 30d, 6m"
            :required="true"
          />
        </div>
      </div>
      <Tabbed :side-tabs="true">
        <Tab
          :weight="4"
          name="data-pods"
          label="Primary Pods"
          tooltip="Primary Pods are responsible for storing the data and running search and indexing operations."
        >
          <DataPods v-model="config.dataNodes" :storage-class-options="storageClassOptions" />
        </Tab>
        <Tab
          :weight="3"
          name="ingest-pods"
          label="Ingest Pods"
          tooltip="Ingest Pods are responsible for running the Opni ingest plugins, as well as indexing data."
        >
          <IngestPods v-model="config.ingestNodes" />
        </Tab>
        <Tab
          :weight="2"
          name="controlplane-pods"
          label="Controlplane Pods"
          tooltip="Controlplane Pods are responsible for maintaining cluster metadata."
        >
          <ControlplanePods v-model="config.controlplaneNodes" :storage-class-options="storageClassOptions" />
        </Tab>
        <Tab
          :weight="1"
          name="dashboard"
          label="Dashboard"
          tooltip="This is responsible for running the OpenSearch Dashboard UI."
        >
          <Dashboard v-model="config.dashboards" />
        </Tab>
      </Tabbed>
    </template>
    <template #details>
      <CapabilityTable name="logging" />
    </template>
  </Backend>
</template>

<style lang="scss" scoped>
::v-deep .tab-container {
  position: relative;
}
</style>

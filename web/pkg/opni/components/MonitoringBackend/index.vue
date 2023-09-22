<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { cloneDeep } from 'lodash';
import { CortexOps, DriverUtil } from '@pkg/opni/api/opni';
import { getClusterStats } from '@pkg/opni/utils/requests';
import { Duration } from '@bufbuild/protobuf';
import Backend from '../Backend';
import CapabilityTable from '../CapabilityTable';
import Grafana from './Grafana';
import StorageComponent from './Storage';

export async function isEnabled() {
  try {
    const state = (await CortexOps.service.Status()).installState;

    return state !== DriverUtil.types.InstallState.NotInstalled;
  } catch (ex) {
    return false;
  }
}

export default {
  components: {
    Backend,
    LabeledSelect,
    Grafana,
    CapabilityTable,
    StorageComponent,
    Tab,
    Tabbed
  },

  data() {
    return {
      presets:           [],
      presetOptions:     [],
      presetIndex:       0,
      config:            null,
      configFromBackend: null
    };
  },

  methods: {
    async updateStatus(capabilities = []) {
      try {
        const stats = await getClusterStats(this);

        capabilities.forEach(c => c.updateStats(stats));
      } catch (ex) {}
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
      await CortexOps.service.Uninstall();
      if (this.config?.cortexConfig?.storage?.s3?.secretAccessKey) {
        this.$set(this.config.cortexConfig.storage.s3, 'secretAccessKey', '');
      }
    },

    async save() {
      if (this.config.cortexConfig.storage.backend === 's3') {
        if (this.config.cortexConfig.storage.s3.endpoint === '') {
          throw new Error('Endpoint is required');
        }

        if (this.config.cortexConfig.storage.s3.bucketName === '') {
          throw new Error('Bucket Name is required');
        }

        if (this.config.cortexConfig.storage.s3.secretAccessKey === '') {
          throw new Error('Secret Access Key is required');
        }
      }

      if (this.config.grafana.enabled) {
        // check if hostname is set and not empty
        if (!this.config.grafana.hostname || this.config.grafana.hostname === '') {
          throw new Error('Grafana hostname is required');
        }
      }

      const newConfig = new CortexOps.types.CapabilityBackendConfigSpec(structuredClone(this.config));
      const activeConfig = await CortexOps.service.GetConfiguration(new DriverUtil.types.GetRequest());
      const shouldSetConfig = activeConfig.revision.revision === 0n;

      const dryRunRequest = new CortexOps.types.DryRunRequest({
        spec:   newConfig,
        action: shouldSetConfig ? DriverUtil.types.Action.Set : DriverUtil.types.Action.Reset,
        target: DriverUtil.types.Target.ActiveConfiguration
      });

      const dryRun = await CortexOps.service.DryRun(dryRunRequest);

      if (dryRun.validationErrors?.length > 0) {
        throw dryRun.validationErrors.map(e => e.message);
      }

      await CortexOps.service.SetDefaultConfiguration(newConfig);

      if (shouldSetConfig) {
        await CortexOps.service.SetConfiguration(new CortexOps.types.CapabilityBackendConfigSpec());
      } else {
        await CortexOps.service.ResetConfiguration(new CortexOps.types.ResetRequest({}));
      }

      await CortexOps.service.Install();
    },

    bannerMessage(status) {
      if (status.warnings?.length > 0) {
        return `There are currently errors that need to be resolved:`;
      }

      switch (status.installState) {
      case DriverUtil.types.InstallState.Uninstalling:
        return `Monitoring is currently uninstalling from the cluster. You can't make changes right now.`;
      case DriverUtil.types.InstallState.Installed:
        return `Monitoring is currently installed on the cluster.`;
      default:
        return `Monitoring is currently in an unknown state on the cluster. You can't make changes right now.`;
      }
    },

    bannerState(status) {
      if (status.warnings?.length > 0) {
        return 'error';
      }

      switch (status.installState) {
      case DriverUtil.types.InstallState.Uninstalling:
        return 'warning';
      case DriverUtil.types.InstallState.Installed:
        return `success`;
      default:
        return `error`;
      }
    },

    isEnabled,

    isUpgradeAvailable() {
      return false;
    },

    async getStatus() {
      try {
        const status = (await CortexOps.service.Status());

        if (status.installState === DriverUtil.types.InstallState.NotInstalled) {
          return null;
        }

        return {
          state:   this.bannerState(status),
          message: this.bannerMessage(status),
          list:     status.warnings
        };
      } catch (ex) {
        return {
          state:   'error',
          message: 'Unable to get status',
          list:     []
        };
      }
    },

    async getConfig() {
      let presets = [];

      try {
        presets = (await CortexOps.service.ListPresets())?.items || [];
      } catch (e) {
        console.error(e);
      }

      this.$set(this, 'presets', presets);
      this.$set(this, 'presetOptions', presets.map((p, i) => ({
        label: p.metadata.displayName,
        value: i
      })));

      this.$set(this, 'configFromBackend', await CortexOps.service.GetDefaultConfiguration(new DriverUtil.types.GetRequest()));
      this.$set(this, 'config', this.configFromBackend);

      if (!this.config.cortexWorkloads) {
        this.applyPreset();
      } else {
        this.prepareConfig();
      }

      return this.config;
    },

    prepareConfig() {
      const configFromBackend = cloneDeep(this.configFromBackend);
      const clone = cloneDeep(this.config || {});

      if (configFromBackend.cortexConfig.storage?.backend === clone.cortexConfig.storage?.backend) {
        this.$set(this, 'config', { ...clone, ...configFromBackend });
      }

      this.$set(this.config, 'cortexWorkloads', this.config.cortexWorkloads || {});
      this.$set(this.config.cortexConfig, 'storage', this.config.cortexConfig.storage || { backend: 's3', s3: {} });
      const backendField = this.config.cortexConfig.storage.backend;

      this.$set(this.config.cortexConfig, 'storage', { ...(clone?.cortexConfig?.storage || {}) });
      this.$set(this.config.cortexConfig.storage, backendField, { ...(clone?.cortexConfig?.storage?.[backendField] || {}) });
      this.$set(this.config.cortexConfig.storage, 'backend', this.config.cortexConfig.storage?.backend || 'filesystem');
      this.$set(this.config, 'grafana', this.config.grafana || { enabled: true, hostname: '' });
      this.$set(this.config.cortexConfig, 'limits', this.config.cortexConfig.limits || { compactorBlocksRetentionPeriod: new Duration({ seconds: BigInt(86400) }) });

      if (this.config.revision?.revision === '0') {
        this.$set(this.config.grafana, 'enabled', true);
        this.$set(this.config.cortexConfig.storage, 'backend', 'filesystem');
      }

      console.log(this.config);
    },

    setPresetAsConfig(index) {
      const config = this.presets[index].spec;

      this.$set(this, 'config', config);
    },

    applyPreset() {
      this.setPresetAsConfig(this.presetIndex);

      this.prepareConfig();
    }
  },
  watch: {
    presetIndex() {
      this.applyPreset();
    }
  }
};
</script>
<template>
  <Backend
    title="Monitoring"
    :is-enabled="isEnabled"
    :disable="disable"
    :is-upgrade-available="isUpgradeAvailable"
    :get-status="getStatus"
    :get-config="getConfig"
    :save="save"
  >
    <template #editing>
      <div class="row mb-20">
        <div class="col span-12">
          <LabeledSelect v-model="presetIndex" :options="presetOptions" label="Preset" />
        </div>
      </div>
      <Tabbed :side-tabs="true">
        <Tab :weight="4" name="storage" label="Storage">
          <StorageComponent v-model="config" :v-if="!!config" />
        </Tab>
        <Tab :weight="3" name="grafana" label="Grafana">
          <Grafana v-model="config.grafana" />
        </Tab>
      </Tabbed>
    </template>
    <template #details>
      <CapabilityTable name="metrics" :header-provider="headerProvider" :update-status-provider="updateStatus" />
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

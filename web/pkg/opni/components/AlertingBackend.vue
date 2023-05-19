<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import {
  InstallState, getClusterConfiguration, configureCluster, getClusterStatus, installCluster, uninstallCluster
} from '../utils/requests/alerts';
import { delay } from '../utils/time';
import Backend from './Backend';

export async function isEnabled() {
  const status = (await getClusterStatus()).state;

  return status !== InstallState.NotInstalled;
}

export default {
  components: {
    Backend,
    LabeledSelect,
  },

  async fetch() {
    try {
      await this.load();
    } catch (ex) {}
  },

  data() {
    return {
      interval:                   null,
      loading:                    false,
      statsInterval:              null,
      modes:                      [
        {
          label:   'Standalone',
          value:   1,
          tooltip: 'This will deploy a single AlertManager instance.'
        },
        {
          label:   'Highly Available',
          value:   3,
          tooltip: 'This will deploy multiple instances of AlertManager in order to improve resilliency.'
        },
      ],
      status:     '',
      config: {
        numReplicas:    1,
        resourceLimits: {
          storage: '500Mi', cpu: '500m', memory: '200Mi'
        }
      }
    };
  },

  methods: {
    async load() {
      this.$set(this, 'config', await getClusterConfiguration());
      this.$set(this.config, 'numReplicas', this.config.numReplicas || 1);
    },

    async disable() {
      await uninstallCluster();
      while (await this.isEnabled()) {
        await delay(3000);
      }
    },

    async save() {
      const status = (await getClusterStatus()).state;

      if (status === InstallState.NotInstalled) {
        await installCluster();
      }

      const config = await getClusterConfiguration();

      while ((await getClusterStatus()).state !== InstallState.Installed) {
        await delay(3000);
      }

      await configureCluster({
        ...config, ...this.config, resourceLimits: { ...this.config.resourceLimits, ...config.resourceLimits }
      });
      this.load();
    },
    bannerMessage(status) {
      switch (status) {
      case InstallState.InstallUpdating:
        return `Alerting is currently updating on the cluster. You can't make changes right now.`;
      case InstallState.Uninstalling:
        return `Alerting is currently uninstalling from the cluster . You can't make changes right now.`;
      case InstallState.Installed:
        return `Alerting is currently installed on the cluster.`;
      default:
        return `Alerting is currently in an unknown state on the cluster. You can't make changes right now.`;
      }
    },

    bannerState(status) {
      switch (status) {
      case InstallState.InstallUpdating:
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
    }
  },
};
</script>
<template>
  <Backend
    title="Alerting"
    :is-enabled="isEnabled"
    :disable="disable"
    :is-upgrade-available="isUpgradeAvailable"
    :get-status="getStatus"
    :save="save"
  >
    <template #editing>
      <div class="row mb-20">
        <div class="col span-12">
          <LabeledSelect v-model="config.numReplicas" :options="modes" label="Mode" />
        </div>
      </div>
    </template>
  </Backend>
</template>

<style lang="scss" scoped>
</style>

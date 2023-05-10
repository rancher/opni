<script>
import RadioGroup from '@components/Form/Radio/RadioGroup';
import {
  getClusterConfiguration, configureCluster, getClusterStatus, installCluster, uninstallCluster
} from '../utils/requests/alerts';
import { delay } from '../utils/time';
import Backend from './Backend';

export async function isEnabled() {
  const status = (await getClusterStatus()).state;

  return status !== 'NotInstalled';
}

export default {
  components: {
    Backend,
    RadioGroup,
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

      if (status === 'NotInstalled') {
        await installCluster();
      }

      const config = await getClusterConfiguration();

      while ((await getClusterStatus()).state !== 'Installed') {
        delay(3000);
      }

      await configureCluster({
        ...config, ...this.config, resourceLimits: { ...this.config.resourceLimits, ...config.resourceLimits }
      });
      this.load();
    },
    bannerMessage(status) {
      switch (status) {
      case 'InstallUpdating':
        return `Alerting is currently updating on the cluster. You can't make changes right now.`;
      case 'Uninstalling':
        return `Alerting is currently uninstalling from the cluster . You can't make changes right now.`;
      case 'Installed':
        return `Alerting is currently installed on the cluster.`;
      default:
        return `Alerting is currently in an unknown state on the cluster. You can't make changes right now.`;
      }
    },

    bannerState(status) {
      switch (status) {
      case 'InstallUpdating':
      case 'Uninstalling':
        return 'warning';
      case 'Installed':
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

        if (status === 'NotInstalled') {
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
          <RadioGroup
            v-model="config.numReplicas"
            name="mode"
            label="Mode"
            :options="modes"
          />
        </div>
      </div>
    </template>
  </Backend>
</template>

<style lang="scss" scoped>
</style>

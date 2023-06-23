<script>
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import isEmpty from 'lodash/isEmpty';
import { Checkbox } from '@components/Form/Checkbox';
import { capitalize } from 'lodash';
import { exceptionToErrorsArray } from '../utils/error';
import {
  getAISettings, getRuntimeClasses, upgrade, isUpgradeAvailable, updateAISettings, deleteAISettings
} from '../utils/requests/aiops';
import AiOpsPretrained from './AiOpsPretrained';

const SENTINEL = '--___SENTINEL___--';

export async function isEnabled() {
  const config = await getAISettings();

  return window.location.search.includes('aiops=true') || !isEmpty(config);
}

export default {
  components: {
    AiOpsPretrained,
    AsyncButton,
    Banner,
    Checkbox,
    Loading,
    LabeledSelect,
  },

  async fetch() {
    try {
      await this.load();
    } catch (ex) {}
  },

  data() {
    return {
      lastConfig:           null,
      interval:                   null,
      error:                      '',
      loading:                    false,
      enabled:                    false,
      runtimeClasses:       [
        { label: 'None', value: SENTINEL }
      ],
      isUpgradeAvailable:   false,
      showAdvanced:         false,
      advancedControlplane: { replicas: 1 },
      advancedRancher:      { replicas: 1 },
      advancedLonghorn:     { replicas: 1 },
      status:               '',
      config:               {
        controlplane: { },
        rancher:      { },
        longhorn:     { }
      }
    };
  },

  methods: {
    async load() {
      const runtimeClasses = (await getRuntimeClasses()).RuntimeClasses.map(r => ({ label: r, value: r }));

      this.$set(this, 'runtimeClasses', [...this.runtimeClasses, ...runtimeClasses]);

      this.$set(this, 'lastConfig', await getAISettings());
      const config = this.lastConfig || {};

      this.$set(this, 'config', { ...config });
      if (config.gpuSettings) {
        this.$set(this.config, 'gpuSettings', { ...config.gpuSettings });
      }
      this.$set(this, 'advancedControlplane', config.controlplane || { replicas: 1 });
      this.$set(this, 'advancedRancher', config.rancher || { replicas: 1 });
      this.$set(this, 'advancedLonghorn', config.longhorn || { replicas: 1 });

      this.$set(this, 'enabled', await isEnabled());
      this.$set(this, 'isUpgradeAvailable', await isUpgradeAvailable());
    },

    enable() {
      this.$set(this, 'enabled', true);
    },

    async disable(buttonCallback) {
      try {
        await deleteAISettings();
        buttonCallback(true);
        this.$set(this, 'enabled', false);
      } catch (err) {
        buttonCallback(false);
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      }
    },

    updatePretrainedBeforeSave(key) {
      if (this.config[key]) {
        const advancedKey = `advanced${ capitalize(key) }`;
        const copy = { ...this[advancedKey] };

        if (!copy.httpSource && typeof copy.httpSource !== 'undefined') {
          delete copy.httpSource;
        }

        if (!copy.imageSource && typeof copy.imageSource !== 'undefined') {
          delete copy.imageSource;
        }

        this.$set(this.config, key, copy);
      }
    },

    async save(buttonCallback) {
      try {
        this.updatePretrainedBeforeSave('controlplane');
        this.updatePretrainedBeforeSave('rancher');
        this.updatePretrainedBeforeSave('longhorn');

        await updateAISettings(this.config);
        await this.load();

        this.$set(this, 'error', '');
        buttonCallback(true);
        this.$router.replace({ name: 'ai-ops' });
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);
      }
    },

    async upgrade() {
      try {
        await upgrade(this.config);
        this.load();

        this.$set(this, 'error', '');
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      }
    },

    toggleAdvanced() {
      this.$set(this, 'showAdvanced', !this.showAdvanced);
    },
  },

  computed: {
    controlPlane: {
      get() {
        return !!this.config.controlplane;
      },

      set(val) {
        this.$set(this.config, 'controlplane', val ? {} : undefined);
      }
    },

    rancher: {
      get() {
        return !!this.config.rancher;
      },

      set(val) {
        this.$set(this.config, 'rancher', val ? {} : undefined);
      }
    },

    longhorn: {
      get() {
        return !!this.config.longhorn;
      },

      set(val) {
        this.$set(this.config, 'longhorn', val ? {} : undefined);
      }
    },

    gpu: {
      get() {
        return !!this.config.gpuSettings;
      },

      set(val) {
        this.$set(this.config, 'gpuSettings', val ? {} : undefined);
      }
    },

    runtimeClass: {
      get() {
        return (this.config.gpuSettings || {}).runtimeClass ? this.config.gpuSettings.runtimeClass : SENTINEL;
      },

      set(val) {
        this.$set(this.config.gpuSettings, 'runtimeClass', val === SENTINEL ? undefined : val);
      }
    },

    runtimeClassDisabled() {
      return this.runtimeClasses.length === 1 || !this.gpu;
    },

    runtimeClassTooltip() {
      return this.runtimeClassDisabled && this.gpu ? `There aren't any Runtime Classes available.` : null;
    },

    hasExistingConfig() {
      return !!this.lastConfig;
    }
  }
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <h1>AIOps</h1>
      <AsyncButton
        v-if="enabled"
        class="btn bg-error"
        mode="edit"
        action-label="Disable"
        waiting-label="Disabling"
        :disabled="!hasExistingConfig"
        @click="disable"
      />
    </header>
    <Banner v-if="enabled && isUpgradeAvailable" color="info">
      <div>There's an upgrade currently available.</div>
      <button class="btn role-primary" @click="upgrade">
        Upgrade
      </button>
    </Banner>
    <div class="body">
      <div v-if="enabled" class="enabled">
        <div class="row">
          <h4>Use GPU</h4>
        </div>
        <div class="row mb-20 border">
          <div class="col span-6 middle">
            <Checkbox v-model="gpu" label="Enable GPU services" />
          </div>
          <div class="col span-6">
            <LabeledSelect v-model="runtimeClass" :options="runtimeClasses" label="Runtime Class" :disabled="runtimeClassDisabled" :tooltip="runtimeClassTooltip" />
          </div>
        </div>
        <div class="row mb-20 border">
          <div class="col span-6">
            <h4 class="mt-0">
              Use Pretrained Models
            </h4>
            <div><Checkbox v-model="controlPlane" label="Controlplane" /></div>
            <div><Checkbox v-model="rancher" label="Rancher" /></div>
            <div><Checkbox v-model="longhorn" label="Longhorn" /></div>
          </div>
        </div>
        <div class="row mb-20">
          <a class="hand block" @click="toggleAdvanced">Show Advanced</a>
        </div>
        <div v-if="showAdvanced">
          <AiOpsPretrained v-model="advancedControlplane" :disabled="!config.controlplane" title="Controlplane" />
          <AiOpsPretrained v-model="advancedRancher" :disabled="!config.rancher" title="Rancher" />
          <AiOpsPretrained v-model="advancedLonghorn" :disabled="!config.longhorn" title="Longhorn" />
        </div>
      </div>
      <div v-else class="not-enabled">
        <h4>AIOps is not currently enabled. Installing it will add additional resources on this cluster.</h4>
        <AsyncButton
          class="btn role-primary"
          mode="edit"
          action-label="Install"
          waiting-label="Installing"
          @click="enable"
        />
      </div>
      <div v-if="enabled" class="resource-footer">
        <n-link class="btn role-secondary mr-10" :to="{name: 'clusters'}">
          Cancel
        </n-link>
        <AsyncButton mode="edit" :disabled="!enabled" @click="save" />
      </div>
      <Banner
        v-if="error"
        color="error"
        :label="error"
      />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.middle {
  display: flex;
  flex-direction: row;
  align-items: center;
}

.banner {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}

.banner-message {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

.border {
    border-bottom: 1px solid #dcdee7;
    padding-bottom: 15px;
}

header {
  width: 100%;
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

::v-deep {
  .card-container {
    min-height: initial;
  }

  .nowrap {
    white-space: nowrap;
  }

  .monospace {
    font-family: $mono-font;
  }

  .cluster-status {
    padding-left: 40px;
  }

  .capability-status {
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
  }

  .not-enabled {
    text-align: center;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    height: 100%;
  }

  .enabled {
    width: 100%;
  }
}
</style>

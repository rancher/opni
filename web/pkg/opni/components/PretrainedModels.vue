<script>
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';
import { Checkbox } from '@components/Form/Checkbox';
import { capitalize } from 'lodash';
import { getAISettings, upgrade, isUpgradeAvailable, updateAISettings } from '../utils/requests/aiops';
import { exceptionToErrorsArray } from '../utils/error';
import AiOpsPretrained from './AiOpsPretrained';
import DependencyWall from './DependencyWall';
import { isEnabled as isAiOpsEnabled } from './LogAnomalyBackend';

export default {
  components: {
    AiOpsPretrained,
    AsyncButton,
    Banner,
    Checkbox,
    DependencyWall,
    Loading,
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
      isUpgradeAvailable:   false,
      showAdvanced:         false,
      advancedControlplane: { replicas: 1 },
      advancedRancher:      { replicas: 1 },
      advancedLonghorn:     { replicas: 1 },
      status:               '',
      isAiOpsEnabled:       false,
      config:               {
        controlplane: { },
        rancher:      { },
        longhorn:     { }
      }
    };
  },

  methods: {
    async load() {
      if (!(await isAiOpsEnabled())) {
        return;
      }
      this.$set(this, 'isAiOpsEnabled', true);

      this.$set(this, 'lastConfig', await getAISettings());
      const config = this.lastConfig || {};

      this.$set(this, 'config', { ...config });
      if (config.gpuSettings) {
        this.$set(this.config, 'gpuSettings', { ...config.gpuSettings });
      }
      this.$set(this, 'advancedControlplane', config.controlplane || { replicas: 1 });
      this.$set(this, 'advancedRancher', config.rancher || { replicas: 1 });
      this.$set(this, 'advancedLonghorn', config.longhorn || { replicas: 1 });

      this.$set(this, 'isUpgradeAvailable', await isUpgradeAvailable());
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
        this.$router.replace({ name: 'pretrained-models' });
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
      <h1>Pretrained Models</h1>
    </header>
    <DependencyWall v-if="!isAiOpsEnabled" title="Pretrained Models" dependency-name="Log Anomaly" route-name="log-anomaly" />
    <div v-else class="body">
      <div class="enabled">
        <div class="row mb-20 border">
          <div class="col span-6">
            <div><Checkbox v-model="controlPlane" label="Controlplane" tooltip="Insights are provided for logs associated with the Kubernetes control plane, optimized for K3s, RKE1, RKE2, and vanilla K8s distributions" /></div>
            <div><Checkbox v-model="rancher" label="Rancher" tooltip="Insights are provided for logs associated with Rancher manager and Rancher agent" /></div>
            <div><Checkbox v-model="longhorn" label="Longhorn" tooltip="Insights are provided for logs associated with Longhorn" /></div>
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
      <div v-if="isAiOpsEnabled" class="resource-footer">
        <n-link class="btn role-secondary mr-10" :to="{name: 'log-anomaly'}">
          Cancel
        </n-link>
        <AsyncButton mode="edit" @click="save" />
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

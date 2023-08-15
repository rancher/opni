<script>
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { S3_REGIONS, S3_ENDPOINT_TO_REGION, S3_REGION_TO_ENDPOINT } from '@pkg/opni/utils/storage';
import { getAISettings, upgrade, updateAISettings, deleteAISettings } from '../utils/requests/aiops';
import { getModelTrainingParameters } from '../utils/requests/workload';
import Backend from './Backend';

export async function isEnabled() {
  const settings = await getAISettings();

  return settings !== null;
}

const DEFAULT_S3_REGION = 'us-east-1';

export default {
  components: {
    Backend, LabeledInput, LabeledSelect, Tab, Tabbed
  },

  async fetch() {
    await this.load();
  },

  data() {
    const storageOptions = ['Internal', 'S3'];

    return {
      error:              '',
      loading:            false,
      enabled:            false,
      config:             { endpoint: S3_REGION_TO_ENDPOINT[DEFAULT_S3_REGION] },
      storageOptions,
      storage:            storageOptions[0],
      regionOptions:      ['Unknown', ...S3_REGIONS],
      region:             DEFAULT_S3_REGION
    };
  },

  computed: {
    storageTooltip() {
      if (this.storage === 'Internal') {
        return "Internal storage isn't recommended for production";
      }

      return undefined;
    }
  },

  methods: {
    isEnabled,

    async load() {
      const settings = await getAISettings();

      if (settings.s3Settings) {
        this.$set(this, 'storage', 'S3');
        this.$set(this, 'config', settings.s3Settings);
        if (this.config.endpoint) {
          const normalizedEndpoint = this.normalizeEndpoint(this.config.endpoint);

          this.$set(this, 'region', S3_ENDPOINT_TO_REGION[normalizedEndpoint] || 'Unknown');
        } else {

        }
      }
    },

    async enable() {
      await this.save();
      this.$router.replace({ name: 'pretrained-models' });
    },

    async disable() {
      const params = await getModelTrainingParameters();

      if (params.items.length > 0) {
        throw new Error('You have to clear the Deployment Watchlist before disabling Log Anomaly.');
      }

      await deleteAISettings();
      this.$router.replace({ name: 'ai-ops' });
    },

    async save() {
      if (this.storage === 'S3') {
        if (this.config.endpoint === '') {
          throw new Error('Endpoint is required');
        }

        if (this.config.accessKey === '') {
          throw new Error('Access Key ID is required');
        }

        // We can't make this check as long as the backend returns empty
        // if (this.config.secretKey === '') {
        //   throw new Error('Secret Access Key is required');
        // }

        if (this.config.nulogBucket === '') {
          throw new Error('NuLog Bucket Name is required');
        }

        if (this.config.drainBucket === '') {
          throw new Error('Drain Bucket Name is required');
        }
      }

      const settings = this.storage === 'S3' ? { s3Settings: this.config } : undefined;

      await updateAISettings(settings);

      this.$set(this, 'error', '');
    },

    async upgrade() {
      await upgrade(this.config);

      this.$set(this, 'error', '');
    },

    normalizeEndpoint(endpoint) {
      return (endpoint || '').replace('https://', '').replace('http://');
    }
  },

  watch: {
    region() {
      const newEndpoint = S3_REGION_TO_ENDPOINT[this.region];

      if (!this.config.endpoint.includes(newEndpoint) && this.region !== 'Unknown') {
        this.$set(this.config, 'endpoint', newEndpoint);
      }
    },

    'config.endpoint'() {
      const normalizedEndpoint = this.normalizeEndpoint(this.config.endpoint);

      this.$set(this, 'region', S3_ENDPOINT_TO_REGION[normalizedEndpoint] || 'Unknown');
    }
  }
};
</script>
<template>
  <Backend
    title="Log Anomaly"
    :is-enabled="isEnabled"
    :disable="disable"
    :save="save"
  >
    <template #editing>
      <Tabbed :side-tabs="true">
        <Tab :weight="1" name="storage" label="Storage">
          <div class="row border" :class="{'bottom': storage === 'S3'}">
            <div class="col span-6">
              <LabeledSelect v-model="storage" label="Type" :options="storageOptions" :tooltip="storageTooltip" />
            </div>
          </div>
          <div v-if="storage === 'S3'">
            <div class="row mt-20">
              <div class="col span-6">
                <LabeledSelect v-model="region" label="Region" :options="regionOptions" />
              </div>
              <div class="col span-6">
                <LabeledInput v-model="config.endpoint" label="Endpoint" :required="true" />
              </div>
            </div>
            <div class="row mt-10">
              <div class="col span-6">
                <LabeledInput v-model="config.accessKey" label="Access Key ID" :required="true" />
              </div>
              <div class="col span-6">
                <LabeledInput v-model="config.secretKey" label="Secret Access Key" :required="true" type="password" />
              </div>
            </div>
            <div class="row mt-10">
              <div class="col span-6">
                <LabeledInput v-model="config.nulogBucket" label="NuLog Bucket Name" :required="true" />
              </div>
              <div class="col span-6">
                <LabeledInput v-model="config.drainBucket" label="Drain Bucket Name" :required="true" />
              </div>
            </div>
          </div>
        </Tab>
      </Tabbed>
    </template>
    <template #details>
      <h4>
        Navigate to <n-link :to="{name: 'pretrained-models'}">
          Pretrained Models
        </n-link> and <n-link :to="{name: 'workload-model-config'}">
          Deployment Watchlist
        </n-link> for further configuration.
      </h4>
    </template>
  </Backend>
</template>

<style lang="scss" scoped>
.bottom {
  border-bottom: 1px solid var(--header-border);
  padding-bottom: 20px;
}
</style>

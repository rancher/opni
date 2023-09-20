<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import AsyncButton from '@shell/components/AsyncButton';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { Banner } from '@components/Banner';
import { exceptionToErrorsArray } from '@pkg/opni/utils/error';
import {
  createAlertEndpoint,
  getAlertEndpoint,
  testAlertEndpoint,
  updateAlertEndpoint,
} from '@pkg/opni/utils/requests/alerts';
import Slack from './Slack';
import Email from './Email';
import PagerDuty from './PagerDuty';
import Webhook from './Webhook';

export default {
  components: {
    AsyncButton,
    Email,
    LabeledInput,
    LabeledSelect,
    Loading,
    PagerDuty,
    Tab,
    Tabbed,
    Banner,
    Webhook,
    Slack,
  },

  async fetch() {
    const endpointRequest =
      this.$route.params.id && this.$route.params.id !== 'create' ? getAlertEndpoint(this.$route.params.id, this) : Promise.resolve(false);

    if (await endpointRequest) {
      const endpoint = await endpointRequest;

      this.$set(this, 'type', endpoint.type);
      this.$set(this.config, 'name', endpoint.nameDisplay);
      this.$set(this.config, 'description', endpoint.description);
      this.$set(this.config.endpoint, endpoint.type, endpoint.endpoint);
    }
  },

  data() {
    const types = [
      {
        label: 'Slack',
        value: 'slack',
      },
      {
        label: 'Email',
        value: 'email',
      },
      {
        label: 'PagerDuty',
        value: 'pagerDuty',
      },
      {
        label: 'Webhook',
        value: 'webhook',
      },
    ];
    const type = types[3].value;

    return {
      error:  '',
      types,
      type,
      config: {
        name:        '',
        description: '',
        endpoint:    {
          slack:     {},
          email:     {},
          pagerDuty: {},
          webhook:   {},
        },
      },
    };
  },

  methods: {
    async save(buttonCallback) {
      if (this.config === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }

      try {
        const config = this.createConfig();

        if (this.$route.params.id && this.$route.params.id !== 'create') {
          const updateConfig = {
            forceUpdate: true,
            id:          { id: this.$route.params.id },
            updateAlert: config,
          };

          await updateAlertEndpoint(updateConfig);
        } else {
          await createAlertEndpoint(config);
        }
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);

        return;
      }

      this.$set(this, 'error', '');
      buttonCallback(true);
      this.$router.replace({ name: 'endpoints' });
    },

    cancel() {
      this.$router.replace({ name: 'endpoints' });
    },

    createConfig() {
      return {
        name:        this.config.name,
        description: this.config.description,
        [this.type]: this.config.endpoint[this.type],
        id:          this.$route.params.id,
      };
    },

    async testEndpoint(buttonCallback) {
      try {
        await testAlertEndpoint({ id: this.$route.params.id });
        this.$set(this, 'error', '');
        buttonCallback(true);
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);
      }
    },
  },

  computed: {
    matchLabelsToSave() {
      return {
        matchLabels:      this.matchLabels,
        matchExpressions: this.matchExpressions,
      };
    },
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <div class="row mb-20">
      <div class="col span-6">
        <LabeledInput v-model="config.name" label="Name" :required="true" />
      </div>
      <div class="col span-6">
        <LabeledInput v-model="config.description" label="Description" />
      </div>
    </div>
    <Tabbed :side-tabs="true" class="mb-20">
      <Tab name="options" label="Options" :weight="3">
        <div class="row" :class="{ bottom: !!type }">
          <div class="col span-12">
            <LabeledSelect v-model="type" label="Type" :options="types" />
          </div>
        </div>
        <Slack
          v-if="type === 'slack'"
          v-model="config.endpoint.slack"
          class="mt-20"
        />
        <Email
          v-if="type === 'email'"
          v-model="config.endpoint.email"
          class="mt-20"
        />
        <PagerDuty
          v-if="type === 'pagerDuty'"
          v-model="config.endpoint.pagerDuty"
          class="mt-20"
        />
        <Webhook
          v-if="type === 'webhook'"
          v-model="config.endpoint.webhook"
          class="mt-20"
        />
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn btn-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <AsyncButton
        class="mr-10"
        mode="edit"
        success-label="Test Succeeded"
        action-label="Test Endpoint"
        waiting-label="Testing Endpoint"
        error-label="Test Failed"
        action-color="btn-secondary"
        @click="testEndpoint"
      />
      <AsyncButton mode="edit" @click="save" />
    </div>
    <Banner v-if="error" color="error" :label="error" />
  </div>
</template>

<style lang="scss" scoped>
.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}

.install-command {
  width: 100%;
}

::v-deep .warning {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}
</style>

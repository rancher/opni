<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { Checkbox } from '@components/Form/Checkbox';
import AsyncButton from '@shell/components/AsyncButton';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { Banner } from '@components/Banner';
import { exceptionToErrorsArray } from '../utils/error';
import { createAlertEndpoint, getAlertEndpoint, testAlertEndpoint, updateAlertEndpoint } from '../utils/requests/alerts';

export default {
  components: {
    AsyncButton,
    Checkbox,
    LabeledInput,
    LabeledSelect,
    Loading,
    Tab,
    Tabbed,
    Banner,
  },

  async fetch() {
    const endpointRequest = this.$route.params.id && this.$route.params.id !== 'create' ? getAlertEndpoint(this.$route.params.id, this) : Promise.resolve(false);

    if (await endpointRequest) {
      const endpoint = await endpointRequest;

      this.$set(this, 'type', endpoint.type);
      this.$set(this.config, 'name', endpoint.nameDisplay);
      this.$set(this.config, 'description', endpoint.description);
      this.$set(this.config.endpoint, endpoint.type, endpoint.endpoint);
    }
  },

  data() {
    return {
      error:            '',
      type:             'slack',
      types:            [
        {
          label: 'Slack',
          value: 'slack'
        },
        {
          label: 'Email',
          value: 'email'
        },
        {
          label: 'PagerDuty',
          value: 'pagerDuty'
        }
      ],
      config: {
        name:        '',
        description: '',
        endpoint:    {
          slack: {}, email: {}, pagerDuty: {}
        }
      }
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
            updateAlert: config
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
        name: this.config.name, description: this.config.description, [this.type]: this.config.endpoint[this.type]
      };
    },

    async testEndpoint() {
      try {
        await testAlertEndpoint({ endpoint: this.createConfig() });
        this.$set(this, 'error', '');
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      }
    }
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
        <LabeledInput
          v-model="config.name"
          label="Name"
          :required="true"
        />
      </div>
      <div class="col span-6">
        <LabeledInput
          v-model="config.description"
          label="Description"
        />
      </div>
    </div>
    <Tabbed :side-tabs="true" class="mb-20">
      <Tab
        name="options"
        label="Options"
        :weight="3"
      >
        <div class="row" :class="{'bottom': !!type}">
          <div class="col span-12">
            <LabeledSelect v-model="type" label="Type" :options="types" />
          </div>
        </div>
        <div v-if="type === 'slack'" class="row mt-20">
          <div class="col span-6">
            <LabeledInput v-model="config.endpoint.slack.webhookUrl" label="Webhook URL" :required="true" />
          </div>
          <div class="col span-6">
            <LabeledInput v-model="config.endpoint.slack.channel" label="Channel" :required="true" />
          </div>
        </div>
        <div v-if="type === 'email'">
          <div class="row mt-20 bottom mb-10">
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.to" label="To Email" :required="true" />
            </div>
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.smtpFrom" label="From Email" :required="true" />
            </div>
          </div>
          <h4>SMTP</h4>
          <div class="row mt-10">
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.smtpSmartHost" label="Smart Host" :required="true" />
            </div>
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.smtpAuthIdentity" label="Identity" :required="true" />
            </div>
          </div>
          <div class="row mt-10">
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.smtpAuthUsername" label="Username" :required="true" />
            </div>
            <div class="col span-6">
              <LabeledInput v-model="config.endpoint.email.smtpAuthPassword" label="Password" type="password" :required="true" />
            </div>
          </div>
          <div class="row mt-10">
            <div class="col span-6">
              <Checkbox v-model="config.endpoint.email.smtpRequireTLS" label="Required TLS" />
            </div>
          </div>
        </div>
        <div v-if="type === 'pagerDuty'" class="row mt-20">
          <div class="col span-12">
            <LabeledInput v-model="config.endpoint.pagerDuty.integrationKey" label="Integration Key" :required="true" />
          </div>
        </div>
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn btn-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <button class="btn btn-info mr-10" @click="testEndpoint">
        Test Endpoint
      </button>
      <AsyncButton mode="edit" @click="save" />
    </div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
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

.bottom {
  border-bottom: 1px solid var(--header-border);
  padding-bottom: 20px;
}
</style>

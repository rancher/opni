<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import AsyncButton from '@shell/components/AsyncButton';
import ArrayList from '@shell/components/form/ArrayList';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { Banner } from '@components/Banner';
import dayjs from 'dayjs';
import {
  createAlertCondition, getAlertConditionGroups, updateAlertCondition, deactivateSilenceAlertCondition, silenceAlertCondition
} from '@pkg/opni/utils/requests/alerts';
import { exceptionToErrorsArray } from '../../utils/error';
import { Severity, SeverityResponseToEnum } from '../../models/alerting/Condition';
import { getClusters } from '../../utils/requests/management';
import AttachedEndpoints, { createDefaultAttachedEndpoints } from '../AttachedEndpoints';
import { createConditionRequest } from './shared';
import AgentDisconnect from './AgentDisconnect';
import KubeState from './KubeState';
import DownstreamCapability from './DownstreamCapability';
import MonitoringBackend from './MonitoringBackend';
import Prometheus from './Prometheus';

export function createDefaultConfig() {
  return {
    name:              '',
    groupId:           '',
    description:       '',
    labels:            [],
    severity:          Severity.INFO,
    attachedEndpoints: createDefaultAttachedEndpoints()
  };
}

export default {
  components: {
    AgentDisconnect,
    KubeState,
    DownstreamCapability,
    MonitoringBackend,
    ArrayList,
    AsyncButton,
    AttachedEndpoints,
    LabeledInput,
    LabeledSelect,
    Loading,
    Tab,
    Tabbed,
    Banner,
    Prometheus,
  },

  async fetch() {
    await this.load();
  },

  data() {
    return {
      AgentDisconnect,
      KubeState,
      DownstreamCapability,
      MonitoringBackend,
      Prometheus,
      conditionTypes: [
        AgentDisconnect.TYPE_OPTION,
        KubeState.TYPE_OPTION,
        DownstreamCapability.TYPE_OPTION,
        Prometheus.TYPE_OPTION,
        MonitoringBackend.TYPE_OPTION,
      ],
      type:              AgentDisconnect.TYPE,
      ...AgentDisconnect.DEFAULT_CONFIG,
      ...KubeState.DEFAULT_CONFIG,
      ...DownstreamCapability.DEFAULT_CONFIG,
      ...Prometheus.DEFAULT_CONFIG,
      ...MonitoringBackend.DEFAULT_CONFIG,
      config:            createDefaultConfig(),
      originalCondition: null,

      options: {
        clusterOptions:  [],
        silenceOptions:         [
          {
            label: '30 minutes',
            value: '30m'
          },
          {
            label: '1 hour',
            value: '1h'
          },
          {
            label: '3 hours',
            value: '3h'
          },
          {
            label: '1 day',
            value: '24h'
          }
        ],
      },
      groups:     [],
      silenceFor: '1h',
      error:      '',
    };
  },

  methods: {
    async load() {
      const conditionRequest = createConditionRequest(this.$route);
      const clusters = await getClusters(this);
      const hasOneMonitoring = clusters.some(c => c.isCapabilityInstalled('metrics'));
      const groups = await getAlertConditionGroups();

      this.$set(this, 'groups', groups.map(g => ({
        value: g.id,
        label: g.id === '' ? 'Default' : g.id
      })));

      if (!hasOneMonitoring) {
        this.conditionTypes.splice(1, 1);
      }

      if (await conditionRequest) {
        const condition = await conditionRequest;

        this.$set(this, 'originalCondition', condition);

        this.$set(this, 'type', condition.type);
        this.$set(this, condition.type, condition.alertType );
        const defaultConfig = createDefaultConfig();

        this.$set(this, 'config', {
          ...defaultConfig,
          ...condition.base.alertCondition,
          attachedEndpoints: {
            ...defaultConfig.attachedEndpoints,
            details: { ...defaultConfig.attachedEndpoints.details, ...(condition.base.alertCondition?.attachedEndpoints?.details || {}) },
            ...(condition.base.alertCondition?.attachedEndpoints || {})
          },
          alertType: undefined
        });

        this.$set(this.config, 'severity', SeverityResponseToEnum[this.config.severity] || Severity.INFO);
      }
    },
    async save(buttonCallback) {
      if (this.name === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }
      try {
        const condition = { ...this.config, alertType: { [this.type]: this[this.type] } };

        if (condition.attachedEndpoints.items.length === 0 || condition.attachedEndpoints.items.every(item => !item?.endpointId)) {
          delete condition.attachedEndpoints;
        }

        // Necessary to work the taggable LabeledSelect because new values will just show up as a label
        if (condition.groupId && typeof condition.groupId === 'object') {
          condition.groupId = condition.groupId.label;
        }

        if (this.originalCondition) {
          const updateConfig = {
            id:          { id: this.originalCondition.id, groupId: this.originalCondition.groupId },
            updateAlert: condition
          };

          await updateAlertCondition(updateConfig);
        } else {
          await createAlertCondition(condition);
        }
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);

        return;
      }
      this.$set(this, 'error', '');
      buttonCallback(true);
      this.$router.replace({ name: 'alarms' });
    },

    cancel() {
      this.$router.replace({ name: 'alarms' });
    },

    async silence() {
      try {
        const request = {
          conditionId: { id: this.originalCondition.id, groupId: this.originalCondition.groupId },
          duration:    this.silenceFor
        };

        await silenceAlertCondition(request);
        this.load();
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      }
    },

    async resume() {
      try {
        if (this.silenceId) {
          await deactivateSilenceAlertCondition({ id: this.originalCondition.id, groupId: this.originalCondition.groupId });
          this.load();
        }
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
      }
    },
  },

  computed: {
    id() {
      return this.$route.params.id && this.$route.params.id !== 'create' ? this.$route.params.id : undefined;
    },

    silenceId() {
      return this.config?.silence?.silenceId;
    },

    matchLabelsToSave() {
      return {
        matchLabels:      this.matchLabels,
        matchExpressions: this.matchExpressions,
      };
    },

    silenceUntil() {
      if (!this.config?.silence?.endsAt) {
        return 'until resumed';
      }

      return dayjs(this.config?.silence?.endsAt || undefined).format('dddd, MMMM D YYYY, h:mm:ss a');
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
        label="Alarm Options"
        :weight="3"
      >
        <div class="row bottom mb-20">
          <div class="col span-6">
            <LabeledSelect v-model="type" label="Type" :options="conditionTypes" :required="true" />
          </div>
          <div class="col span-6">
            <LabeledSelect v-model="config.groupId" label="Group" :options="groups" :taggable="true" :searchable="true" />
          </div>
        </div>
        <AgentDisconnect v-if="type === AgentDisconnect.TYPE" v-model="system" />
        <KubeState v-if="type === KubeState.TYPE" v-model="kubeState" />
        <DownstreamCapability v-if="type === DownstreamCapability.TYPE" v-model="downstreamCapability" />
        <MonitoringBackend v-if="type === MonitoringBackend.TYPE" v-model="monitoringBackend" />
        <Prometheus v-if="type === Prometheus.TYPE" v-model="prometheusQuery" />
      </Tab>
      <Tab
        name="messaging"
        label="Message Options"
        :weight="2"
      >
        <AttachedEndpoints v-model="config.attachedEndpoints" :show-severity="true" :severity="config.severity" @severity="(val) => config.severity = val" />
      </Tab>
      <Tab
        name="tags"
        label="Tags"
        :weight="1"
      >
        <div class="row">
          <div class="col span-12">
            <ArrayList v-model="config.labels" add-label="Add Tag" :read-allowed="false" />
          </div>
        </div>
      </Tab>
      <Tab
        v-if="id"
        name="silence"
        label="Silence"
        :weight="1"
      >
        <div v-if="silenceId" class="row">
          <div class="col span-12 middle">
            Silenced until {{ silenceUntil }}
            <button class="btn role-primary ml-10" @click="resume">
              Resume Now
            </button>
          </div>
        </div>
        <div v-else class="row">
          <div class="col span-4">
            <LabeledSelect v-model="silenceFor" :options="options.silenceOptions" label="Silence For" />
          </div>
          <div class="col span-3 middle">
            <button class="btn role-primary" @click="silence">
              Silence Now
            </button>
          </div>
        </div>
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn btn-secondary mr-10" @click="cancel">
        Cancel
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

.middle {
  display: flex;
  flex-direction: row;
  align-items: center;
}
</style>

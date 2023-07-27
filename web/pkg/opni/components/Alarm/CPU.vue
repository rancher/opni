<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import AsyncButton from '@shell/components/AsyncButton';
import ArrayList from '@shell/components/form/ArrayList';
import ArrayListSelect from '@shell/components/form/ArrayListSelect';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { Banner } from '@components/Banner';
import dayjs from 'dayjs';
import { createComputedTime } from '@pkg/opni/utils/computed';
import { AlertType, Severity, SeverityResponseToEnum } from '../models/alerting/Condition';
import {
  createAlertCondition, getAlertConditionChoices, getAlertCondition, updateAlertCondition, deactivateSilenceAlertCondition, silenceAlertCondition
} from '../utils/requests/alerts';
import { getClusters } from '../utils/requests/management';
import { exceptionToErrorsArray } from '../utils/error';
import AttachedEndpoints, { createDefaultAttachedEndpoints } from '../AttachedEndpoints';

export function createDefaultConfig() {
  return {
    name:              '',
    description:       '',
    labels:            [],
    severity:          Severity.INFO,
    attachedEndpoints: createDefaultAttachedEndpoints()
  };
}

export default {
  components: {
    ArrayList,
    ArrayListSelect,
    AsyncButton,
    AttachedEndpoints,
    LabeledInput,
    LabeledSelect,
    Loading,
    Tab,
    Tabbed,
    UnitInput,
    Banner,
  },

  async fetch() {
    await Promise.all([this.loadChoices(), this.loadClusters()]);

    await this.load();
  },

  data() {
    return {
      conditionTypes: [
        {
          label: 'Agent Disconnect',
          value: 'system'
        },
        {
          label: 'Kube State',
          value: 'kubeState'
        },
        {
          label: 'Downstream Capability',
          value: 'downstreamCapability'
        },
        {
          label: 'Monitoring Backend',
          value: 'monitoringBackend'
        },
        {
          label: 'CPU',
          value: 'cpu'
        },
        {
          label: 'Memory',
          value: 'memory'
        },
        {
          label: 'Filesystem',
          value: 'fs'
        },
      ],
      type: 'system',

      system: {
        choices: { clusters: [] },
        config:  { clusterId: { id: '' }, timeout: '30s' }
      },
      kubeState: {
        choices: { clusters: [] },
        config:  {
          clusterId: '', objectType: '', objectName: '', namespace: '', state: '', for: '30s'
        }
      },
      downstreamCapability: {
        choices: { clusters: [], states: [] },
        config:  {
          clusterId: '', capabilityState: [], for: '30s'
        }
      },
      monitoringBackend: {
        choices: { },
        config:  { }
      },
      cpu: {
        choices: { },
        config:  { }
      },
      memory: {
        choices: { },
        config:  { }
      },
      fs: {
        choices: { },
        config:  { }
      },
      config: createDefaultConfig(),

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
      silenceFor: '1h',
      error:      '',
    };
  },

  methods: {
    async load() {
      const conditionRequest = this.$route.params.id && this.$route.params.id !== 'create' ? getAlertCondition(this.$route.params.id, this) : Promise.resolve(false);
      const clusters = await getClusters(this);
      const hasOneMonitoring = clusters.some(c => c.isCapabilityInstalled('metrics'));

      if (!hasOneMonitoring) {
        this.conditionTypes.splice(1, 1);
      }

      if (await conditionRequest) {
        const condition = await conditionRequest;

        this.$set(this, 'type', condition.type);
        this.$set(this[condition.type], 'config', condition.alertType );
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
        const condition = { ...this.config, alertType: { [this.type]: this[this.type].config } };

        if (condition.attachedEndpoints.items.length === 0 || condition.attachedEndpoints.items.every(item => !item?.endpointId)) {
          delete condition.attachedEndpoints;
        }

        if (this.$route.params.id && this.$route.params.id !== 'create') {
          const updateConfig = {
            id:          { id: this.$route.params.id },
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

    async loadChoices() {
      try {
        const map = {
          system:               AlertType.SYSTEM,
          kubeState:            AlertType.KUBE_STATE,
          composition:          AlertType.COMPOSITION,
          controlFlow:          AlertType.CONTROL_FLOW,
          downstreamCapability: AlertType.DOWNSTREAM_CAPABILTIY,
          monitoringBackend:    AlertType.MONITORING_BACKEND,
          cpu:                  AlertType.CPU,
          memory:               AlertType.MEMORY,
          fs:                   AlertType.FS,
        };

        const allChoices = await getAlertConditionChoices({ alertType: map[this.type] });
        const choices = allChoices[this.type];

        this.$set(this[this.type], 'choices', { ...choices });
      } catch (ex) {}
    },

    async loadClusters() {
      const clusters = await getClusters(this);

      this.$set(this.options, 'clusterOptions', clusters.map(c => ({
        label: c.nameDisplay,
        value: c.id
      })));
    },

    async silence() {
      try {
        const request = {
          conditionId: { id: this.id },
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
          await deactivateSilenceAlertCondition(this.id);
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

    systemClusterOptions() {
      const options = this.options.clusterOptions;

      if (!this.options.clusterOptions.find(o => o.value === this.system.config.clusterId.id)) {
        this.$set(this.system.config.clusterId, 'id', options[0]?.value || '');
      }

      return options;
    },

    kubeStateClusterOptions() {
      const options = this.options.clusterOptions;

      if (!this.options.clusterOptions.find(o => o.value === this.kubeState.config.clusterId)) {
        this.$set(this.kubeState.config, 'clusterId', options[0]?.value || '');
      }

      return options;
    },

    kubeStateObjectTypeOptions() {
      if (!this.kubeState.config.clusterId) {
        return [];
      }

      const options = Object.keys(this.kubeState.choices.clusters[this.kubeState.config.clusterId]?.resourceTypes || {});

      if (!options.find(o => o === this.kubeState.config.objectType)) {
        this.$set(this.kubeState.config, 'objectType', options[0] || '');
      }

      return options;
    },

    kubeStateNamespaceOptions() {
      if (!this.kubeState.config.objectType) {
        return [];
      }

      const options = Object.keys(this.kubeState.choices.clusters[this.kubeState.config.clusterId]?.resourceTypes?.[this.kubeState.config.objectType].namespaces || {});

      if (!options.find(o => o === this.kubeState.config.namespace)) {
        this.$set(this.kubeState.config, 'namespace', options[0] || '');
      }

      return options;
    },

    kubeStateObjectNameOptions() {
      if (!this.kubeState.config.namespace) {
        return [];
      }

      const options = this.kubeState.choices.clusters[this.kubeState.config.clusterId]?.resourceTypes?.[this.kubeState.config.objectType].namespaces?.[this.kubeState.config.namespace].objects || [];

      if (!options.find(o => o === this.kubeState.config.objectName)) {
        this.$set(this.kubeState.config, 'objectName', options[0] || '');
      }

      return options;
    },

    kubeStateStateOptions() {
      const options = this.kubeState.choices.states || [];

      if (!options.find(o => o === this.kubeState.config.state)) {
        this.$set(this.kubeState.config, 'state', options[0] || '');
      }

      return options;
    },

    systemTimeout: createComputedTime('system.config.timeout'),

    kubeStateFor: createComputedTime('kubeState.config.for'),

    silenceUntil() {
      if (!this.config?.silence?.endsAt) {
        return 'until resumed';
      }

      return dayjs(this.config?.silence?.endsAt || undefined).format('dddd, MMMM D YYYY, h:mm:ss a');
    },
    downstreamCapabilityClusterOptions() {
      const options = this.options.clusterOptions;

      if (!this.options.clusterOptions.find(o => o.value === this.downstreamCapability.config.clusterId)) {
        this.$set(this.downstreamCapability.config, 'clusterId', options[0]?.value || '');
      }

      return options;
    },

    downstreamCapabilityFor: createComputedTime('downstreamCapability.config.for'),

    downstreamCapabilityStateOptions() {
      const options = this.downstreamCapability.choices.clusters[this.downstreamCapability.config.clusterId]?.states || [];

      if (!options.find(o => o === this.downstreamCapability.config.state)) {
        this.$set(this.downstreamCapability.config, 'capabilityState', [options[0]] || []);
      }

      return options;
    },
    monitoringBackendBackendComponentOptions() {
      const options = this.monitoringBackend.choices.backendComponents || [];

      if (!options.find(o => o === this.monitoringBackend.config.backendComponents)) {
        this.$set(this.monitoringBackend.config, 'backendComponents', [options[0]] || []);
      }

      return options;
    },

    monitoringBackendFor: createComputedTime('monitoringBackend.config.for'),
  },

  watch: {
    type() {
      this.loadChoices();
    }
  }
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
        <div class="row bottom">
          <div class="col span-12">
            <LabeledSelect v-model="type" label="Type" :options="conditionTypes" :required="true" />
          </div>
        </div>
        <div v-if="type === 'system'" class="row mt-20">
          <div class="col span-6">
            <LabeledSelect v-model="system.config.clusterId.id" label="Cluster" :options="systemClusterOptions" :required="true" />
          </div>
          <div class="col span-6">
            <UnitInput v-model="systemTimeout" label="Timeout" suffix="s" :required="true" />
          </div>
        </div>
        <div v-if="type === 'kubeState'">
          <h4 class="mt-20">
            Target Metric
          </h4>
          <div class="row mt-10">
            <div class="col span-12">
              <LabeledSelect v-model="kubeState.config.clusterId" label="Cluster" :options="kubeStateClusterOptions" :required="true" />
            </div>
          </div>
          <div class="row mt-20">
            <div class="col span-6">
              <LabeledSelect v-model="kubeState.config.objectType" label="Object Type" :disabled="kubeStateObjectTypeOptions.length === 0" :options="kubeStateObjectTypeOptions" :required="true" />
            </div>
            <div class="col span-6">
              <LabeledSelect v-model="kubeState.config.namespace" label="Namespace" :disabled="kubeStateNamespaceOptions.length === 0" :options="kubeStateNamespaceOptions" :required="true" />
            </div>
          </div>
          <div class="row mt-10">
            <div class="col span-6">
              <LabeledSelect v-model="kubeState.config.objectName" label="Object Name" :disabled="kubeStateObjectNameOptions.length === 0" :options="kubeStateObjectNameOptions" :required="true" />
            </div>
          </div>
          <h4 class="mt-20">
            Threshold
          </h4>
          <div class="row mt-10">
            <div class="col span-6">
              <LabeledSelect v-model="kubeState.config.state" label="State" :disabled="kubeStateStateOptions.length === 0" :options="kubeStateStateOptions" :required="true" />
            </div>
            <div class="col span-6">
              <UnitInput v-model="kubeStateFor" label="Duration" suffix="s" :required="true" />
            </div>
          </div>
        </div>
        <div v-if="type === 'downstreamCapability'">
          <h4 class="mt-20">
            Target Metric
          </h4>
          <div class="row mt-10">
            <div class="col span-6">
              <LabeledSelect v-model="downstreamCapability.config.clusterId" label="Cluster" :options="downstreamCapabilityClusterOptions" :required="true" />
            </div>
            <div class="col span-6">
              <UnitInput v-model="downstreamCapabilityFor" label="Duration" suffix="s" :required="true" />
            </div>
          </div>
          <div class="row mt-10">
            <div class="col span-12">
              <ArrayListSelect
                v-model="downstreamCapability.config.capabilityState"
                label="State"
                :disabled="downstreamCapabilityStateOptions.length === 0"
                :options="downstreamCapabilityStateOptions"
                :required="true"
                add-label="Add State"
              />
            </div>
          </div>
        </div>
        <div v-if="type === 'monitoringBackend'">
          <h4 class="mt-20">
            Target Metric
          </h4>
          <div class="row mt-10">
            <div class="col span-6">
              <UnitInput v-model="monitoringBackendFor" label="Duration" suffix="s" :required="true" />
            </div>
          </div>
          <div class="row mt-10">
            <div class="col span-12">
              <ArrayListSelect
                v-model="monitoringBackend.config.backendComponents"
                label="Backend Components"
                :disabled="monitoringBackendBackendComponentOptions.length === 0"
                :options="(monitoringBackendBackendComponentOptions)"
                :required="true"
                add-label="Add Backend Component"
              />
            </div>
          </div>
        </div>
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

<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import Loading from '@shell/components/Loading';
import { AlertType } from '../../models/alerting/Condition';
import { loadClusters, loadChoices } from './shared';

const TYPE = 'kubeState';

const CONSTS = {
  TYPE,
  ENUM:        AlertType.KUBE_STATE,
  TYPE_OPTION: {
    label: 'Kube State',
    value: 'kubeState'
  },
  DEFAULT_CONFIG: {
    [TYPE]: {
      clusterId: '', objectType: '', objectName: '', namespace: '', state: '', for: '30s'
    }
  },
};

export default {
  ...CONSTS,

  components: {
    LabeledSelect,
    Loading,
    UnitInput,
  },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  async fetch() {
    await Promise.all([this.loadChoices(), this.loadClusters()]);
  },

  data() {
    return {
      ...CONSTS,
      clusterOptions: [],
      clusters:       [],
      choices:        { clusters: [] },
    };
  },

  methods: {
    async loadChoices() {
      await loadChoices(this, this.TYPE, this.ENUM);
    },

    async loadClusters() {
      await loadClusters(this);
    },
  },

  computed: {
    kubeStateClusterOptions() {
      const options = this.clusterOptions;

      if (!options.find(o => o.value === this.value.clusterId)) {
        this.$set(this.value, 'clusterId', options[0]?.value || '');
      }

      return options;
    },

    kubeStateObjectTypeOptions() {
      if (!this.value.clusterId) {
        return [];
      }

      const options = Object.keys(this.choices.clusters[this.value.clusterId]?.resourceTypes || {});

      if (!options.find(o => o === this.value.objectType)) {
        this.$set(this.value, 'objectType', options[0] || '');
      }

      return options;
    },

    kubeStateNamespaceOptions() {
      if (!this.value.objectType) {
        return [];
      }

      const options = Object.keys(this.choices.clusters[this.value.clusterId]?.resourceTypes?.[this.value.objectType].namespaces || {});

      if (!options.find(o => o === this.value.namespace)) {
        this.$set(this.value, 'namespace', options[0] || '');
      }

      return options;
    },

    kubeStateObjectNameOptions() {
      if (!this.value.namespace) {
        return [];
      }

      const options = this.choices.clusters[this.value.clusterId]?.resourceTypes?.[this.value.objectType].namespaces?.[this.value.namespace].objects || [];

      if (!options.find(o => o === this.value.objectName)) {
        this.$set(this.value, 'objectName', options[0] || '');
      }

      return options;
    },

    kubeStateStateOptions() {
      const options = this.choices.states || [];

      if (!options.find(o => o === this.value.state)) {
        this.$set(this.value, 'state', options[0] || '');
      }

      return options;
    },

    kubeStateFor: {
      get() {
        return Number.parseInt(this.value.for || '0');
      },

      set(value) {
        this.$set(this.value, 'for', `${ (value || 0) }s`);
      }
    },
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
    <h4 class="mt-20">
      Kubernetes Object
    </h4>
    <div class="row mt-10">
      <div class="col span-12">
        <LabeledSelect v-model="value.clusterId" label="Cluster" :options="kubeStateClusterOptions" :required="true" />
      </div>
    </div>
    <div class="row mt-20">
      <div class="col span-6">
        <LabeledSelect v-model="value.objectType" label="Object Type" :disabled="kubeStateObjectTypeOptions.length === 0" :options="kubeStateObjectTypeOptions" :required="true" />
      </div>
      <div class="col span-6">
        <LabeledSelect v-model="value.namespace" label="Namespace" :disabled="kubeStateNamespaceOptions.length === 0" :options="kubeStateNamespaceOptions" :required="true" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-6">
        <LabeledSelect v-model="value.objectName" label="Object Name" :disabled="kubeStateObjectNameOptions.length === 0" :options="kubeStateObjectNameOptions" :required="true" />
      </div>
    </div>
    <h4 class="mt-20">
      Threshold
    </h4>
    <div class="row mt-10">
      <div class="col span-6">
        <LabeledSelect v-model="value.state" label="State" :disabled="kubeStateStateOptions.length === 0" :options="kubeStateStateOptions" :required="true" />
      </div>
      <div class="col span-6">
        <UnitInput v-model="kubeStateFor" label="Duration" suffix="s" :required="true" />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
</style>

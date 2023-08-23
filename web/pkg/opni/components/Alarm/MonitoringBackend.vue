<script>
import ArrayListSelect from '@shell/components/form/ArrayListSelect';
import UnitInput from '@shell/components/form/UnitInput';
import Loading from '@shell/components/Loading';
import { AlertType } from '../../models/alerting/Condition';
import { loadClusters, loadChoices } from './shared';

const TYPE = 'monitoringBackend';

export const CONSTS = {
  TYPE,
  ENUM:        AlertType.MONITORING_BACKEND,
  TYPE_OPTION: {
    label: 'Monitoring Backend',
    value: TYPE,
    enum:  AlertType.MONITORING_BACKEND
  },
  DEFAULT_CONFIG: { [TYPE]: { clusterId: { id: '' }, for: '30s' } },
};

export default {
  ...CONSTS,

  components: {
    ArrayListSelect,
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
      choices:        { clusters: [] },
      error:          '',
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
    monitoringBackendBackendComponentOptions() {
      const options = this.choices.backendComponents || [];

      if ((!this.value.backendComponents || this.value.backendComponents.length === 0) && !options.find(o => o === this.value.backendComponents)) {
        this.$set(this.value, 'backendComponents', options[0] ? [options[0]] : []);
      }

      return options;
    },

    monitoringBackendFor: {
      get() {
        return Number.parseInt(this.value.for || '0');
      },

      set(value) {
        this.$set(this.value, 'for', `${ (value || 0) }s`);
      }
    }
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <h4 class="mt-20">
      State
    </h4>
    <div class="row mt-10">
      <div class="col span-6">
        <UnitInput v-model="monitoringBackendFor" label="Duration" suffix="s" :required="true" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-12">
        <ArrayListSelect
          v-model="value.backendComponents"
          label="Backend Components"
          :disabled="monitoringBackendBackendComponentOptions.length === 0"
          :options="(monitoringBackendBackendComponentOptions)"
          :required="true"
          add-label="Add Backend Component"
        />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
</style>

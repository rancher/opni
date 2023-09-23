<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import TextAreaAutoGrow from '@components/Form/TextArea/TextAreaAutoGrow';
import Loading from '@shell/components/Loading';
import { createComputedTime } from '@pkg/opni/utils/computed';
import { AlertType } from '@pkg/opni/models/alerting/Condition';
import { mapClusterOptions, loadChoices } from './shared';

const TYPE = 'prometheusQuery';

export const CONSTS = {
  TYPE,
  ENUM:        AlertType.PROMETHEUS_QUERY,
  TYPE_OPTION: {
    label: 'Prometheus',
    value: TYPE,
    enum:  AlertType.PROMETHEUS_QUERY
  },
  DEFAULT_CONFIG: {
    [TYPE]: {
      clusterId: { id: '' }, for: '30s', query: ''
    }
  },
};

export default {
  ...CONSTS,

  components: {
    LabeledSelect,
    Loading,
    UnitInput,
    TextAreaAutoGrow
  },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  async fetch() {
    await this.loadChoices();
  },

  data() {
    return {
      choices:        { clusters: [] },
      error:          '',
    };
  },

  methods: {
    async loadChoices() {
      await loadChoices(this, this.TYPE, this.ENUM);
    },
  },

  computed: {
    ...mapClusterOptions(),

    prometheusQueryClusterOptions() {
      const options = this.clusterOptions;

      if (!this.clusterOptions.find(o => o.value === this.value.clusterId.id)) {
        this.$set(this.value.clusterId, 'id', options[0]?.value || '');
      }

      return options;
    },

    prometheusQueryFor: createComputedTime('value.for', 60),
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <div class="row mt-20">
      <div class="col span-6">
        <LabeledSelect v-model="value.clusterId.id" label="Cluster" :options="prometheusQueryClusterOptions" :required="true" />
      </div>
      <div class="col span-6">
        <UnitInput v-model="prometheusQueryFor" label="Duration" suffix="mins" :required="true" />
      </div>
    </div>
    <div class="row mt-20">
      <div class="col span-12">
        <h4>
          Query
        </h4>
        <TextAreaAutoGrow v-model="value.query" :min-height="250" :required="true" />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
</style>

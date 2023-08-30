<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import Loading from '@shell/components/Loading';
import { createComputedTime } from '@pkg/opni/utils/computed';
import { AlertType } from '../../models/alerting/Condition';
import { loadClusters, loadChoices } from './shared';

const TYPE = 'system';

export const CONSTS = {
  TYPE,
  ENUM:        AlertType.SYSTEM,
  TYPE_OPTION: {
    label: 'Agent Disconnect',
    value: TYPE,
    enum:  AlertType.SYSTEM
  },
  DEFAULT_CONFIG: { [TYPE]: { clusterId: { id: '' }, timeout: '30s' } },
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
    systemClusterOptions() {
      const options = this.clusterOptions;

      if (!options.find(o => o.value === this.value.clusterId.id)) {
        this.$set(this.value.clusterId, 'id', options[0]?.value || '');
      }

      return options;
    },

    systemTimeout: createComputedTime('value.timeout'),
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else class="row">
    <div class="col span-6">
      <LabeledSelect v-model="value.clusterId.id" label="Cluster" :options="systemClusterOptions" :required="true" />
    </div>
    <div class="col span-6">
      <UnitInput v-model="systemTimeout" label="Timeout" suffix="s" :required="true" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
</style>

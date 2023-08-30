<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { getAlertConditionGroups } from '@pkg/opni/utils/requests/alerts';
import { CONDITION_TYPES } from '@pkg/opni/utils/alarms';
import { getClusters } from '@pkg/opni/utils/requests/management';

const NO_FILTER_SENTINEL = '__-NO_FILTER_SENTINEL-__';
const NO_FILTER = {
  value: NO_FILTER_SENTINEL,
  label: 'All'
};
const DEFAULT_GROUP = '';

export function createDefaults() {
  const options = createDefaultOptions();

  return {
    options,
    itemFilter: getItemFilter(options, createDefaultFilters())
  };
}

function createDefaultOptions() {
  return {
    group:      [],
    cluster:    [],
    type:       [NO_FILTER, ...CONDITION_TYPES],
  };
}

function createDefaultFilters() {
  return {
    group:       DEFAULT_GROUP,
    cluster:     NO_FILTER_SENTINEL,
    type:        NO_FILTER_SENTINEL,
  };
}

export async function loadOptions() {
  const [groups, clusters] = await Promise.all([getAlertConditionGroups(), getClusters(this)]);

  const groupOptions = groups.map(g => ({
    value: g.id,
    label: g.id === DEFAULT_GROUP ? 'Default' : g.id
  }));

  const clusterOptions = clusters.map(c => ({
    value: c.id,
    label: c.nameDisplay,
  }));

  return {
    group:   [NO_FILTER, ...groupOptions],
    cluster: [NO_FILTER, ...clusterOptions],
    type:    [NO_FILTER, ...CONDITION_TYPES]
  };
}

export function getItemFilter(options, filters) {
  const groupIds = filters.group === NO_FILTER_SENTINEL ? options.group.map(f => f.value) : [filters.group];
  const clusters = filters.cluster === NO_FILTER_SENTINEL ? undefined : [filters.cluster];
  const alertTypes = filters.type === NO_FILTER_SENTINEL ? undefined : [filters.type];

  return {
    clusters, groupIds, alertTypes
  };
}

export default {
  components: { LabeledSelect },

  props: {
    options: {
      type:     Object,
      required: true
    },
  },

  data() {
    return { filters: createDefaultFilters() };
  },

  methods: {
    updateFilters() {
      this.$emit('item-filter-changed', getItemFilter(this.options, this.filters));
    },

    resetFilters() {
      this.$set(this, 'filters', createDefaultFilters());
      this.updateFilters();
    }
  },
  watch: {
    'filters.cluster'() {
      this.updateFilters();
    },
    'filters.type'() {
      this.updateFilters();
    },
    'filters.group'() {
      this.updateFilters();
    }
  }
};
</script>
<template>
  <div class="condition-filter">
    <LabeledSelect
      v-model="filters.cluster"
      class="cluster mr-10"
      label="Cluster"
      :searchable="true"
      :options="options.cluster"
    />
    <LabeledSelect
      v-model="filters.type"
      :reduce="(option) => option['enum']"
      class="type mr-10"
      label="Type"
      :searchable="true"
      :options="options.type"
    />
    <LabeledSelect
      v-model="filters.group"
      class="group mr-10"
      label="Group"
      :searchable="true"
      :options="options.group"
    />
    <button class="btn btn-sm role-secondary" @click="resetFilters">
      Reset Filters
    </button>
  </div>
</template>

<style lang="scss" scoped>
.condition-filter {
  display: flex;
  flex-direction: row;
  align-items: center;
}
</style>

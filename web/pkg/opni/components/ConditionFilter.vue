<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { getAlertConditionGroups } from '@pkg/opni/utils/requests/alerts';
import { CONDITION_TYPES } from '@pkg/opni/utils/alarms';
import { mapGetters } from 'vuex';

const NO_FILTER_SENTINEL = '__-NO_FILTER_SENTINEL-__';
const NO_FILTER = {
  value: NO_FILTER_SENTINEL,
  label: 'All'
};
const DEFAULT_GROUP = '';

export function createDefaultFilters() {
  const defaults = defaultFilters();

  return getItemFilter([], defaults);
}

function defaultFilters() {
  return {
    group:       DEFAULT_GROUP,
    cluster:     NO_FILTER_SENTINEL,
    type:        NO_FILTER_SENTINEL,
  };
}

function getItemFilter(groupOptions, filters) {
  const groupIds = filters.group === NO_FILTER_SENTINEL ? groupOptions.map(f => f.value).filter(v => v !== NO_FILTER_SENTINEL) : [filters.group];
  const clusters = filters.cluster === NO_FILTER_SENTINEL ? undefined : [filters.cluster];
  const alertTypes = filters.type === NO_FILTER_SENTINEL ? undefined : [filters.type];

  return {
    clusters, groupIds, alertTypes
  };
}

export default {
  components: { LabeledSelect },

  props: {},

  async fetch() {
    const groups = await getAlertConditionGroups();

    this.$set(this, 'groups', groups);
  },

  data() {
    return {
      filters: defaultFilters(),
      groups:  []
    };
  },

  methods: {
    updateFilters() {
      this.$emit('item-filter-changed', this.getItemFilter(this.groupOptions, this.filters));
    },

    resetFilters() {
      this.$set(this, 'filters', defaultFilters());
      this.updateFilters();
    },
    getItemFilter
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
  },
  computed: {
    ...mapGetters({ clusters: 'opni/clusters' }),
    clusterOptions() {
      const clusterOptions = this.clusters.map(c => ({
        value: c.id,
        label: c.nameDisplay,
      }));

      return [NO_FILTER, ...clusterOptions];
    },

    typeOptions() {
      return [NO_FILTER, ...CONDITION_TYPES];
    },

    groupOptions() {
      const groups = this.groups.length === 0 ? [{ id: DEFAULT_GROUP }] : this.groups;
      const groupOptions = groups.map(g => ({
        value: g.id,
        label: g.id === DEFAULT_GROUP ? 'Default' : g.id
      }));

      return [NO_FILTER, ...groupOptions];
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
      :options="clusterOptions"
    />
    <LabeledSelect
      v-model="filters.type"
      :reduce="(option) => option['enum']"
      class="type mr-10"
      label="Type"
      :searchable="true"
      :options="typeOptions"
    />
    <LabeledSelect
      v-model="filters.group"
      class="group mr-10"
      label="Group"
      :searchable="true"
      :options="groupOptions"
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

<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { getClusters } from '@pkg/opni/utils/requests/management';
import CloneToClustersDialog from '@pkg/opni/components/dialogs/CloneToClustersDialog';
import { InstallState, getClusterStatus, getAlertConditionsWithStatus } from '@pkg/opni/utils/requests/alerts';
import LoadingSpinnerOverlay from '@pkg/opni/components/LoadingSpinnerOverlay';
import ConditionFilter, { createDefaults as createConditionFilterDefaults, loadOptions as loadConditionFilterOptions } from '@pkg/opni/components/ConditionFilter';

export default {
  components: {
    CloneToClustersDialog, ConditionFilter, Loading, LoadingSpinnerOverlay, SortableTable
  },
  async fetch() {
    await this.load();
    await this.updateStatuses();
  },

  data() {
    return {
      conditionFilter:        createConditionFilterDefaults(),
      clusters:               [],
      loading:                false,
      loadingTable:           false,
      statsInterval:          null,
      conditions:             [],
      isAlertingEnabled:      false,
      headers:                [
        {
          name:          'status',
          labelKey:      'opni.tableHeaders.status',
          value:         'status',
          formatter:     'StatusBadge',
          width:     100
        },
        {
          name:          'nameDisplay',
          labelKey:      'opni.tableHeaders.name',
          value:         'nameDisplay',
          width:    250
        },
        {
          name:          'description',
          labelKey:      'opni.tableHeaders.description',
          value:         'description',
          width:    250
        },
        {
          name:      'type',
          labelKey:  'opni.tableHeaders.type',
          value:     'typeDisplay'
        },
        {
          name:      'group',
          labelKey:  'opni.tableHeaders.group',
          value:     'groupDisplay'
        },
      ]
    };
  },

  created() {
    this.$on('remove', this.onRemove);
    this.$on('clone', this.onClone);
    this.statsInterval = setInterval(this.updateStatuses, 10000);
  },

  beforeDestroy() {
    this.$off('remove');
    this.$off('clone');
    if (this.statsInterval) {
      clearInterval(this.statsInterval);
    }
  },

  methods: {
    onRemove() {
      this.load();
    },

    onClone(alarm) {
      this.$refs.dialog.open(alarm, alarm.clusterId);
    },

    async load() {
      try {
        this.loading = true;

        const status = (await getClusterStatus()).state;
        const isAlertingEnabled = status === InstallState.Installed || status === InstallState.Updating;

        this.$set(this, 'isAlertingEnabled', isAlertingEnabled);

        if (!isAlertingEnabled) {
          return;
        }

        const [conditionFilterOptions, clusters] = await Promise.all([loadConditionFilterOptions(), getClusters(this)]);

        this.$set(this, 'clusters', clusters);
        this.$set(this.conditionFilter, 'options', conditionFilterOptions);

        await this.updateStatuses();
      } finally {
        this.loading = false;
      }
    },
    async updateStatuses() {
      this.$set(this, 'conditions', await getAlertConditionsWithStatus(this, this.clusters, this.conditionFilter.itemFilter));
    },

    async itemFilterChanged(itemFilter) {
      try {
        this.$set(this, 'loadingTable', true);
        this.$set(this.conditionFilter, 'itemFilter', itemFilter);
        await this.updateStatuses();
      } finally {
        this.$set(this, 'loadingTable', false);
      }
    },
  },
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <div class="title">
        <h1>Alarms</h1>
      </div>
      <div v-if="isAlertingEnabled" class="actions-container">
        <n-link class="btn role-primary" :to="{name: 'alarm-create'}">
          Create
        </n-link>
      </div>
    </header>
    <LoadingSpinnerOverlay v-if="isAlertingEnabled" :loading="loadingTable">
      <SortableTable
        :rows="conditions"
        :headers="headers"
        default-sort-by="expirationDate"
        :search="false"
        key-field="id"
        group-by="clusterDisplay"
        :rows-per-page="15"
      >
        <template #group-by="{group: thisGroup}">
          <div v-trim-whitespace class="group-tab">
            Cluster: {{ thisGroup.ref }}
          </div>
        </template>
        <template #header-right>
          <ConditionFilter :options="conditionFilter.options" @item-filter-changed="itemFilterChanged" />
        </template>
      </SortableTable>
    </LoadingSpinnerOverlay>
    <div v-else class="not-enabled">
      <h4>
        Alerting must be enabled to use Alarms. <n-link :to="{name: 'alerting'}">
          Click here
        </n-link> to enable alerting.
      </h4>
    </div>
    <CloneToClustersDialog ref="dialog" :clusters="clusters" @save="load" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .labeled-select {
    position: relative;
    text-align: left;
    right: 0;
  }

  .search {
    align-items: center;
    justify-content: initial;
  }

  .group {
    width: 260px;
  }

  .type {
    width: 160px;
  }
  .cluster {
    width: 260px;
  }

  .btn-sm {
    height: 40px;
  }

  .bulk {
    display: flex;
    flex-direction: row;
    align-items: center;
  }

  .loading-spinner-container {
    top: 81px;
    z-index: 100;
  }
}

.not-enabled {
  text-align: center;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  height: 100%;
}
</style>

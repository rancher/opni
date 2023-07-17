<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { getClusters } from '@pkg/opni/utils/requests/management';
import CloneToClustersDialog from '@pkg/opni/components/dialogs/CloneToClustersDialog';
import { InstallState, getClusterStatus, getAlertConditionsWithStatus, getAlertConditionGroups } from '@pkg/opni/utils/requests/alerts';
import LoadingSpinner from '@pkg/opni/components/LoadingSpinner';

export default {
  components: {
    CloneToClustersDialog, LabeledSelect, Loading, LoadingSpinner, SortableTable
  },
  async fetch() {
    await this.load();
    await this.updateStatuses();
  },

  data() {
    return {
      clusters:          [],
      loading:           false,
      loadingTable:      false,
      statsInterval:     null,
      conditions:        [],
      isAlertingEnabled: false,
      groupFilter:       '', // Default group
      groupOptions:      [],
      headers:           [
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

        const [groups, clusters] = await Promise.all([getAlertConditionGroups(), getClusters(this)]);

        this.$set(this, 'groupOptions', groups.map(g => ({
          value: g.id,
          label: g.id === '' ? 'Default' : g.id
        })));
        this.$set(this, 'clusters', clusters);

        await this.updateStatuses();
      } finally {
        this.loading = false;
      }
    },
    async updateStatuses() {
      this.$set(this, 'conditions', await getAlertConditionsWithStatus(this, this.clusters, [this.groupFilter]));
    }
  },
  watch: {
    async groupFilter() {
      try {
        this.$set(this, 'loadingTable', true);
        await this.updateStatuses();
      } finally {
        this.$set(this, 'loadingTable', false);
      }
    }
  }
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
    <div v-if="isAlertingEnabled" class="table-container">
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
          <LabeledSelect v-model="groupFilter" label="Group" :searchable="true" :options="groupOptions" />
        </template>
      </SortableTable>
      <div v-if="loadingTable" class="loading-spinner-container">
        <LoadingSpinner />
      </div>
    </div>
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
    &,.v-select {
      width: 300px;
    }
  }

  .bulk {
    display: flex;
    flex-direction: row;
    align-items: center;
  }

  .initial-load-spinner {
      border-color: rgb(219, 136, 240);
      border-top-color: var(--primary);
    }
}

.table-container {
  position: relative;
}

.loading-spinner-container {
  position: absolute;
  left: 0;
  right: 0;
  top: 81px;
  bottom: 0;

  display: flex;
  justify-content: center;
  align-items: center;

  z-index: 100;

  background-color: rgba(242, 242, 242, 0.6);
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

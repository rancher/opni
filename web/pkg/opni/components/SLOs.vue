<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { getClusterStatus } from '../utils/requests/alerts';
import { getSLOs } from '../utils/requests/slo';
import { getClusters } from '../utils/requests/management';
import CloneToClustersDialog from './dialogs/CloneToClustersDialog';

export default {
  components: {
    CloneToClustersDialog, Loading, SortableTable
  },
  async fetch() {
    await this.load();
    await this.updateStatuses();
  },

  data() {
    return {
      loading:             false,
      statsInterval:       null,
      clusters:            [],
      slos:                [],
      hasOneMonitoring:  false,
      isAlertingEnabled: false,
      headers:             [
        {
          name:          'status',
          labelKey:      'tableHeaders.status',
          value:         'status',
          formatter:     'StatusBadge',
          width:     100
        },
        {
          name:          'nameDisplay',
          labelKey:      'tableHeaders.name',
          value:         'nameDisplay',
          width:         undefined
        },
        {
          name:          'tags',
          labelKey:      'tableHeaders.tags',
          value:         'tags',
          formatter:     'ListBubbles'
        },
        {
          name:      'period',
          labelKey:  'tableHeaders.period',
          value:     'period'
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

    onClone(slo) {
      this.$refs.dialog.open(slo, slo.clusterId);
    },

    async load() {
      try {
        this.loading = true;
        const status = (await getClusterStatus()).state;
        const isAlertingEnabled = status === 'Installed';

        this.$set(this, 'isAlertingEnabled', isAlertingEnabled);

        if (!isAlertingEnabled) {
          return;
        }

        const clusters = await getClusters(this);

        this.$set(this, 'clusters', clusters);
        const hasOneMonitoring = clusters.some(c => c.isCapabilityInstalled('metrics'));

        this.$set(this, 'hasOneMonitoring', hasOneMonitoring);

        if (!hasOneMonitoring) {
          return;
        }

        this.$set(this, 'slos', await getSLOs(this, clusters));
        await this.updateStatuses();
      } finally {
        this.loading = false;
      }
    },
    async updateStatuses() {
      const promises = this.slos.map(slo => slo.updateStatus());

      await Promise.all(promises);
    }
  },
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <div class="title">
        <h1>SLOs</h1>
      </div>
      <div v-if="isAlertingEnabled && hasOneMonitoring" class="actions-container">
        <n-link class="btn role-primary" :to="{name: 'slo-create'}">
          Create
        </n-link>
      </div>
    </header>
    <SortableTable
      v-if="isAlertingEnabled && hasOneMonitoring"
      :rows="slos"
      :headers="headers"
      :search="false"
      group-by="clusterNameDisplay"
      default-sort-by="expirationDate"
      key-field="id"
    >
      <template #group-by="{group: thisGroup}">
        <div v-trim-whitespace class="group-tab">
          Cluster: {{ thisGroup.ref }}
        </div>
      </template>
    </SortableTable>
    <div v-else-if="!isAlertingEnabled" class="not-enabled">
      <h4>
        Alerting must be enabled to use SLOs. <n-link :to="{name: 'alerting-backend'}">
          Click here
        </n-link> to enable alerting.
      </h4>
    </div>
    <div v-else class="not-enabled">
      <h4>
        At least one cluster must have Monitoring installed to use SLOs. <n-link :to="{name: 'clusters'}">
          Click here
        </n-link> to enable Monitoring.
      </h4>
    </div>
    <CloneToClustersDialog ref="dialog" :clusters="clusters" @save="load" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .nowrap {
    white-space: nowrap;
  }

  .monospace {
    font-family: $mono-font;
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

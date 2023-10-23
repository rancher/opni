<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { LoggingAdmin } from '@pkg/opni/api/opni';
import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';
import { isEnabled as isLoggingEnabledCheck } from './LoggingBackend';

export default {
  components: { Loading, SortableTable },
  async fetch() {
    await this.load();
  },

  data() {
    return {
      loading:             false,
      snapshots:           [],
      isLoggingEnabled: false,
      headers:             [
        {
          name:          'status',
          labelKey:      'opni.tableHeaders.status',
          value:         'status',
          formatter:     'StatusBadge',
          width:     100
        },
        {
          name:            'nameDisplay',
          labelKey:        'opni.tableHeaders.name',
          value:           'nameDisplay',
        },
        {
          name:            'lastUpdated',
          labelKey:        'opni.tableHeaders.lastUpdated',
          value:           'lastUpdated',
          width:     175
        },
      ]
    };
  },

  created() {
    GlobalEventBus.$on('remove', this.onRemove);
  },

  beforeDestroy() {
    GlobalEventBus.$off('remove');
  },

  methods: {
    onRemove() {
      this.load();
    },

    onClone() {
      this.load();
    },

    async load() {
      try {
        this.loading = true;

        const isLoggingEnabled = await isLoggingEnabledCheck();

        this.$set(this, 'isLoggingEnabled', isLoggingEnabled);

        if (isLoggingEnabled) {
          const snapshotStatuses = await LoggingAdmin.Service.ListSnapshotSchedules();

          console.log(snapshotStatuses);

          this.$set(this, 'snapshots', snapshotStatuses.statuses || []);
        }
      } finally {
        this.loading = false;
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
        <h1>Snapshots</h1>
      </div>
      <div v-if="isLoggingEnabled" class="actions-container">
        <n-link class="btn role-primary" :to="{name: 'snapshot-create'}">
          Create
        </n-link>
      </div>
    </header>
    <SortableTable
      v-if="isLoggingEnabled"
      :rows="snapshots"
      :headers="headers"
      :search="false"
      default-sort-by="expirationDate"
      key-field="id"
      :rows-per-page="15"
    />
    <div v-else class="not-enabled">
      <h4>
        Logging must be enabled to use Snapshots. <n-link :to="{name: 'logging-config'}">
          Click here
        </n-link> to enable logging.
      </h4>
    </div>
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

<script>
import SortableTable from '@shell/components/SortableTable';
import Drawer from '@shell/components/Drawer';
import { Banner } from '@components/Banner';
import Loading from '@shell/components/Loading';

export const LOG_HEADERS = [
  {
    name:      'collapse',
    labelKey:  'opni.tableHeaders.empty',
    formatter: 'Collapse',
    value:     'timestamp',
    sort:      false,
    width:     '20px'
  },
  {
    name:      'date',
    labelKey:  'opni.tableHeaders.date',
    formatter: 'Date',
    value:     'timestamp',
    sort:      false,
    width:     '300px'
  },
  {
    name:          'namespace',
    labelKey:      'opni.tableHeaders.namespace',
    value:         'kubernetesNamespaceName',
    sort:          false,
    width:         '300px'
  },
  {
    name:          'podName',
    labelKey:      'opni.tableHeaders.podName',
    value:         'kubernetesPodName',
    sort:          false
  }
];

export default {
  components: {
    Banner, Drawer, Loading, SortableTable
  },

  props: {
    fromTo: {
      type:     Object,
      required: true,
    },

    logGetter: {
      type:     Function,
      required: true
    }
  },

  data() {
    return {
      LOG_HEADERS,
      showSuspicious:    true,
      showAnomaly:       true,
      showWorkloads:     true,
      showControlPlanes: true,
      logs:              null,
      isOpen:            false,
      lastFilter:        null,
      selectedLevel:     null,
      selectedRow:       null,
    };
  },

  methods: {
    getColor(log) {
      return log.anomalyLevel === 'Anomaly' ? 'error' : 'warning';
    },

    async loadLogs() {
      const logs = await this.logGetter(this.selectedLevel, this.selectedRow, this.logs?.scrollId);

      if (this.logs) {
        this.logs.logs.push(...logs.logs);
        this.logs.scrollId = logs.scrollId;
      } else {
        this.logs = logs;
      }
    },

    open({ level, row }) {
      this.$set(this, 'isOpen', true);
      this.$set(this, 'selectedLevel', level);
      this.$set(this, 'selectedRow', row);
      this.$set(this, 'logs', null);

      this.loadLogs();
    },

    close() {
      this.$set(this, 'isOpen', false);
    }
  },
};
</script>
<template>
  <Drawer :open="isOpen" @close="close">
    <template #title>
      <div class="p-5 pb-0">
        <h1>{{ t('opni.logsDrawer.title') }}</h1>
      </div>
    </template>
    <Loading v-if="logs === null" mode="relative" />
    <div v-else class="contents">
      <div class="row detail">
        <div class="col span-12 p-5 pr-20">
          <SortableTable
            class="table"
            :rows="logs.logs || []"
            :headers="LOG_HEADERS"
            :search="false"
            :table-actions="false"
            :row-actions="false"
            paging="async"
            :paging-async-count="logs.count"
            :paging-async-load-next-page="loadLogs"
            :rows-per-page="100"
            default-sort-by="name"
            key-field="id"
          >
            <template #sub-row="{row, fullColspan}">
              <tr class="opni sub-row">
                <td v-if="row.open" :colspan="fullColspan" class="pt-0">
                  <Banner
                    class="m-0"
                    :color="getColor(row)"
                  >
                    {{ row.log }}
                  </Banner>
                </td>
              </tr>
            </template>
          </SortableTable>
        </div>
      </div>
    </div>
  </drawer>
</template>

<style lang="scss" scoped>
.contents {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  right: 0;

  overflow: hidden;
}

.chart {
  width: calc(100% - 30px - 1.75%);
}

.detail {
  height: 100%;

  .col:nth-of-type(2) {
    border-left: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    padding-left: calc(20px + 1.75%);
  }

  .col {
    overflow-y: scroll;
    padding-bottom: 30px;
  }

  ::v-deep {
    .anomaly {
      color: var(--error);
    }

    .suspicious {
      color: var(--warning);
    }

    .sortable-table {
      table-layout: fixed;
    }
  }
}

h1, h3 {
  display: inline-block;
}
</style>

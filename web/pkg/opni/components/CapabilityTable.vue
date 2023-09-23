<script>
import { Card } from '@components/Card';
import SortableTable from '@shell/components/SortableTable';
import { Banner } from '@components/Banner';
import LoadingSpinner from '@pkg/opni/components/LoadingSpinner';
import { mapGetters } from 'vuex';
import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';
import { Capability } from '@pkg/opni/models/Capability';
import { MetricCapability } from '@pkg/opni/models/MetricCapability';
import UninstallCapabilitiesDialog from '@pkg/opni/components/dialogs/UninstallCapabilitiesDialog';
import CancelUninstallCapabilitiesDialog from '@pkg/opni/components/dialogs/CancelUninstallCapabilitiesDialog';

export default {
  components: {
    Banner,
    Card,
    CancelUninstallCapabilitiesDialog,
    LoadingSpinner,
    SortableTable,
    UninstallCapabilitiesDialog,
  },

  props: {
    name: {
      type:     String,
      required: true
    },

    updateStatusProvider: {
      type:     Function,
      default: () => {}
    },

    headerProvider: {
      type:    Function,
      default: x => x
    }
  },

  async fetch() {
    await this.loadStatus();
    this.createCapabilities();
  },

  data() {
    const headers = [
      {
        name:          'status',
        labelKey:      'opni.tableHeaders.status',
        sort:          ['status.message'],
        value:         'status',
        formatter:     'StatusBadge'
      },
      {
        name:          'nameDisplay',
        labelKey:      'opni.tableHeaders.agentName',
        sort:          ['clusterNameDisplay'],
        value:         'clusterNameDisplay',
        formatter:     'TextWithClass',
        formatterOpts: {
          getClass(row, value) {
            // Value could either be a cluster ID in a UUID format or a
            // friendly name set by the user, if available. If the value is
            // a cluster ID, display it in a monospace font.
            // This regex will match UUID versions 1-5.
            const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

            return uuidRegex.test(value) ? 'monospace' : '';
          }
        }
      }
    ];

    return {
      loading:        false,
      statusInterval: null,
      headers:        this.headerProvider(headers),
      capabilities:   []
    };
  },

  computed: { ...mapGetters({ clusters: 'opni/clusters' }) },

  watch: {
    'capabilities.length'() {
      this.loadStatus();
    },

    clusters: {
      deep: true,
      handler() {
        this.createCapabilities();
      }
    }
  },

  created() {
    GlobalEventBus.$on('uninstallCapabilities', this.openUninstallCapabilitiesDialog);
    GlobalEventBus.$on('cancelUninstallCapabilities', this.openCancelUninstallCapabilitiesDialog);
    this.statusInterval = setInterval(this.loadStatus, 10000);
  },

  beforeDestroy() {
    GlobalEventBus.$off('uninstallCapabilities');
    GlobalEventBus.$off('cancelUninstallCapabilities', this.openCancelUninstallCapabilitiesDialog);
    if (this.statusInterval) {
      clearInterval(this.statusInterval);
    }
  },

  methods: {
    createCapabilities() {
      const currentCapabilities = this.capabilities;
      const newCapabilities = this.clusters.map((cluster) => {
        const foundCapability = currentCapabilities.find(capability => capability.id === cluster.id);

        if (foundCapability) {
          return foundCapability;
        }

        return this.name === 'metrics' ? MetricCapability.createExtended(cluster, this.vue) : Capability.create(this.name, cluster, this.vue);
      });

      this.$set(this, 'capabilities', newCapabilities);
    },
    openUninstallCapabilitiesDialog(capabilities) {
      this.$refs.uninstallCapabilitiesDialog.open(capabilities);
    },

    openCancelUninstallCapabilitiesDialog(cluster, capabilities) {
      this.$refs.cancelCapabilitiesDialog.open(cluster, capabilities);
    },

    async loadStatus() {
      const status = Promise.all(this.capabilities.map(c => c.updateCabilityLogs()));

      if (this.updateStatusProvider) {
        await this.updateStatusProvider(this.capabilities);
      }
      await status;
    },

    async onDialogSave() {
      this.$refs.uninstallCapabilitiesDialog.close(false);
      this.$refs.cancelCapabilitiesDialog.close(false);

      // Reset the interval so we don't have an untimely update
      if (this.statusInterval) {
        clearInterval(this.statusInterval);
      }
      this.statusInterval = setInterval(this.loadStatus, 10000);
    },
  },
};
</script>
<template>
  <div>
    <Card :show-highlight-border="false" :show-actions="false" class="m-0" title="Capability Management">
      <div slot="title">
        <h4 class="ml-10 mb-5">
          Capability Management
        </h4>
      </div>
      <div slot="body" class="p-10">
        <LoadingSpinner v-if="$fetchState.pending" />
        <SortableTable
          :class="{none: $fetchState.pending}"
          :rows="capabilities"
          :headers="headers"
          :search="false"
          :sub-rows="true"
          default-sort-by="nameDisplay"
          key-field="id"
          :rows-per-page="15"
        >
          <template #sub-row="{row, fullColspan}">
            <tr v-if="row.status.state === 'error' || row.status.state === 'warning'" class="sub-row">
              <td :colspan="fullColspan">
                <Banner class="sub-banner m-0" :label="row.status.message" :color="row.status.state" />
              </td>
            </tr>
          </template>
        </SortableTable>
      </div>
    </Card>
    <UninstallCapabilitiesDialog ref="uninstallCapabilitiesDialog" @save="onDialogSave" />
    <CancelUninstallCapabilitiesDialog ref="cancelCapabilitiesDialog" @save="onDialogSave" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .none {
    display: none;
  }

  .main-row {
    border-top: 1px solid var(--sortable-table-top-divider);
  }
}
</style>

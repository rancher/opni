<script>
import SortableTable from '@shell/components/SortableTable';
import { Banner } from '@components/Banner';
import { mapGetters } from 'vuex';
import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';
import CapabilityButton from '@pkg/opni/components/CapabilityButton';
import EditClusterDialog from '@pkg/opni/components/dialogs/EditClusterDialog';
import CantDeleteClusterDialog from '@pkg/opni/components/dialogs/CantDeleteClusterDialog';
import UninstallCapabilitiesDialog from '@pkg/opni/components/dialogs/UninstallCapabilitiesDialog';

export default {
  components: {
    CapabilityButton,
    CantDeleteClusterDialog,
    EditClusterDialog,
    SortableTable,
    Banner,
    UninstallCapabilitiesDialog
  },

  computed: { ...mapGetters({ clusters: 'opni/clusters' }) },

  data() {
    return {
      loading:                      false,
      statsInterval:                null,
      isMonitoringBackendInstalled: false,
      isLoggingBackendInstalled:    false,
      closeStreams:                 null,
      headers:                      [
        {
          name:          'status',
          labelKey:      'opni.tableHeaders.status',
          sort:          ['status.message'],
          value:         'status',
          formatter:     'StatusBadge'
        },
        {
          name:          'nameDisplay',
          labelKey:      'opni.tableHeaders.name',
          sort:          ['nameDisplay'],
          value:         'nameDisplay',
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
        },
        {
          name:          'id',
          labelKey:      'opni.tableHeaders.clusterId',
          tooltip:       'Derived from kube-system namespace',
          sort:          ['id'],
          value:         'id',
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
        },
        {
          name:          'isLocal',
          labelKey:      'opni.tableHeaders.local',
          sort:          ['isLocal'],
          value:         'localIcon',
          formatter:     'Icon',
          width:     20
        },
      ]
    };
  },

  created() {
    GlobalEventBus.$on('cantDeleteCluster', this.openCantDeleteClusterDialog);
    GlobalEventBus.$on('uninstallCapabilities', this.openUninstallCapabilitiesDialog);
    GlobalEventBus.$on('promptEdit', this.openEditDialog);
    GlobalEventBus.$on('copy', this.copyClusterID);
  },

  beforeDestroy() {
    GlobalEventBus.$off('cantDeleteCluster');
    GlobalEventBus.$off('uninstallCapabilities');
    GlobalEventBus.$off('promptEdit');
    GlobalEventBus.$off('copy');
  },

  methods: {
    openEditDialog(cluster) {
      this.$refs.dialog.open(cluster);
    },

    openUninstallCapabilitiesDialog(cluster, capabilities) {
      this.$refs.uninstallCapabilitiesDialog.open(cluster, capabilities);
    },

    openCantDeleteClusterDialog(cluster) {
      this.$refs.cantDeleteClusterDialog.open(cluster);
    },

    copyClusterID(cluster) {
      this.$copyText(cluster.id);
    },

    cancelCapabilityUninstall(cluster, capabilities) {
      cluster.clearCapabilityStatus(capabilities);
    },

    promptRemove(resources) {
      this.vue.$store.commit('action-menu/togglePromptRemove', resources, { root: true });
    },

    load() {
      this.loading = false;
    },

    async onDialogSave() {
      // this.$set(this, 'clusters', await getClusters(this));
      await this.loadStats();

      this.$refs.capabilitiesDialog.close(false);
      this.$refs.cancelCapabilitiesDialog.close(false);
    }
  },
};
</script>
<template>
  <div>
    <header>
      <div class="title">
        <h1>Agents</h1>
      </div>
      <div class="actions-container">
        <n-link class="btn role-primary" :to="{ name: 'agent-create' }">
          Add
        </n-link>
      </div>
    </header>
    <SortableTable
      :rows="clusters"
      :headers="headers"
      :search="false"
      default-sort-by="expirationDate"
      key-field="id"
      :sub-rows="true"
      :rows-per-page="15"
    >
      <template #col:capabilities="{row}">
        <td>
          <CapabilityButton label="Monitoring" type="metrics" :cluster="row" :is-backend-installed="isMonitoringBackendInstalled" />
          <CapabilityButton label="Logging" type="logs" :cluster="row" :is-backend-installed="isLoggingBackendInstalled" />
        </td>
      </template>
      <template #sub-row="{row, fullColspan}">
        <tr v-if="row.status.state === 'error' || row.status.state === 'warning'" class="sub-row">
          <td :colspan="fullColspan">
            <Banner class="sub-banner m-0" :label="row.status.message" :color="row.status.state" />
          </td>
        </tr>
        <tr v-if="row.displayLabels.length > 0" class="sub-row">
          <td :colspan="fullColspan" class="cluster-status">
            Labels:
            <span v-for="label in row.displayLabels" :key="label" class="bubble ml-5">
              {{ label }}
            </span>
          </td>
        </tr>
      </template>
    </SortableTable>
    <EditClusterDialog ref="dialog" @save="load" />
    <CantDeleteClusterDialog ref="cantDeleteClusterDialog" />
    <UninstallCapabilitiesDialog ref="uninstallCapabilitiesDialog" @save="load" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .main-row {
    border-top: 1px solid var(--sortable-table-top-divider);
  }

  .sub-row {
    &, td {
      border-bottom: none;
    }
  }

  .nowrap {
    white-space: nowrap;
  }

  .monospace {
    font-family: $mono-font;
  }

  .cluster-status {
    padding-left: 40px;
  }

  .capability-status {
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
  }
}
</style>

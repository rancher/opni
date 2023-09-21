<script>
import { mapGetters } from 'vuex';
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';
import { getRoles } from '../utils/requests/management';
import AddTokenDialog from './dialogs/AddTokenDialog';

export default {
  components: {
    AddTokenDialog, Loading, SortableTable
  },
  async fetch() {
    await this.load();
  },

  data() {
    return {
      loading:  false,
      roles:    [],
      headers:  [
        {
          name:      'nameDisplay',
          labelKey:  'opni.tableHeaders.name',
          sort:      ['nameDisplay'],
          value:     'nameDisplay',
          width:     undefined
        },
        {
          name:          'clusterNames',
          labelKey:      'opni.tableHeaders.clusters',
          sort:          ['clusterNames'],
          value:         'clusterNames',
          formatter:     'List'
        },
        {
          name:          'matchLabels',
          labelKey:      'opni.tableHeaders.matchLabels',
          sort:          ['matchLabels'],
          value:         'matchLabelsDisplay',
          formatter:     'ListBubbles'
        },
        {
          name:          'matchExpressions',
          labelKey:      'opni.tableHeaders.matchExpressions',
          sort:          ['matchExpressions'],
          value:         'matchExpressionsDisplay',
          formatter:     'ListBubbles'
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

    openCreateDialog(ev) {
      ev.preventDefault();
      this.$refs.dialog.open();
    },

    async load() {
      try {
        this.loading = true;
        const roles = await getRoles(this);

        this.$set(this, 'roles', roles);
      } finally {
        this.loading = false;
      }
    }
  },

  computed: { ...mapGetters({ clusters: 'opni/clusters' }) }
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <div class="title">
        <h1>Roles</h1>
      </div>
      <div class="actions-container">
        <n-link class="btn role-primary" :to="{ name: 'role-create' }">
          Create
        </n-link>
      </div>
    </header>
    <SortableTable
      :rows="roles"
      :headers="headers"
      :search="false"
      default-sort-by="expirationDate"
      key-field="id"
      :rows-per-page="15"
    />
    <AddTokenDialog ref="dialog" @save="load" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .nowrap {
    white-space: nowrap;
  }
}
</style>

<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { getClusters, getRoles } from '../utils/requests/management';
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
      clusters: [],
      headers:  [
        {
          name:      'nameDisplay',
          labelKey:  'tableHeaders.name',
          sort:      ['nameDisplay'],
          value:     'nameDisplay',
          width:     undefined
        },
        {
          name:          'clusterNames',
          labelKey:      'tableHeaders.clusters',
          sort:          ['clusterNames'],
          value:         'clusterNames',
          formatter:     'List'
        },
        {
          name:          'matchLabels',
          labelKey:      'tableHeaders.matchLabels',
          sort:          ['matchLabels'],
          value:         'matchLabelsDisplay',
          formatter:     'ListBubbles'
        },
        {
          name:          'matchExpressions',
          labelKey:      'tableHeaders.matchExpressions',
          sort:          ['matchExpressions'],
          value:         'matchExpressionsDisplay',
          formatter:     'ListBubbles'
        },
      ]
    };
  },

  created() {
    this.$on('remove', this.onClusterDelete);
  },

  beforeDestroy() {
    this.$off('remove');
  },

  methods: {
    onClusterDelete() {
      this.load();
    },

    openCreateDialog(ev) {
      ev.preventDefault();
      this.$refs.dialog.open();
    },

    async load() {
      try {
        this.loading = true;
        const [roles, clusters] = await Promise.all([getRoles(this), getClusters(this)]);

        roles.forEach(role => role.setClusters(clusters));

        this.$set(this, 'roles', roles);
        this.$set(this, 'clusters', clusters);
      } finally {
        this.loading = false;
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

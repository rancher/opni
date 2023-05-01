<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { getRoleBindings } from '../utils/requests/management';
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
      loading:      false,
      roleBindings:  [],
      headers:      [
        {
          name:      'name',
          labelKey:  'tableHeaders.name',
          sort:      ['name'],
          value:     'name',
          width:     undefined
        },
        {
          name:          'subjects',
          labelKey:      'tableHeaders.subjects',
          sort:          ['subjects'],
          value:         'subjects',
          formatter:     'ListBubbles'
        },
        {
          name:          'role',
          labelKey:      'tableHeaders.role',
          sort:          ['role'],
          value:         'role',
        }
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
        await this.$set(this, 'roleBindings', await getRoleBindings(this));
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
        <h1>Role Bindings</h1>
      </div>
      <div class="actions-container">
        <n-link class="btn role-primary" :to="{ name: 'role-binding-create' }">
          Create
        </n-link>
      </div>
    </header>
    <SortableTable
      :rows="roleBindings"
      :headers="headers"
      :search="false"
      default-sort-by="expirationDate"
      key-field="name"
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

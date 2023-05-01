<script>
import { Card } from '@components/Card';
import { Banner } from '@components/Banner';

export default {
  components: {
    Banner,
    Card,
  },
  data() {
    return { cluster: null };
  },
  methods: {
    close() {
      this.$modal.hide('cant-delete-cluster-dialog');
    },

    open(cluster) {
      this.$set(this, 'cluster', cluster);
      this.$modal.show('cant-delete-cluster-dialog');
    },

    uninstall(ev) {
      ev.preventDefault();
      this.close();
      this.cluster.uninstallCapabilities();
    }
  },

  computed: {
    capabilities() {
      return this.cluster?.capabilities || [];
    },

    clusterName() {
      return this.cluster?.nameDisplay;
    }
  }
};
</script>

<template>
  <modal
    ref="cantDeleteClusterDialog"
    class="cant-delete-cluster-dialog"
    name="cant-delete-cluster-dialog"
    styles="background-color: var(--nav-bg); border-radius: var(--border-radius); max-height: 90vh;"
    height="auto"
    width="600"
    :scrollable="true"
    @closed="close()"
  >
    <Card
      class="edit-cluster-card"
      :show-highlight-border="false"
    >
      <div slot="body" class="pt-10">
        <h4 class="text-default-text pt-4" v-html="`You can't delete <b>${clusterName}</b> yet`" />
        <div class="row">
          <div class="col span-12">
            <Banner color="warning">
              You can't delete a cluster while capbabilities are installed. This cluster currently has {{ capabilities.join(' and ') }} installed. You can uninstall the capabilities <a href="#" @click="uninstall">here</a>.
            </Banner>
          </div>
        </div>
      </div>
      <div slot="actions" class="buttons">
        <button class="btn role-primary mr-10" @click="close">
          {{ t("generic.ok") }}
        </button>
      </div>
    </Card>
  </modal>
</template>
<style lang='scss' scoped>
.edit-cluster-card {
  margin: 0;

  ::v-deep {
    .kv-container {
      max-height: 500px;
      overflow-y: auto;
      padding-right: 10px;
    }

    .kv-item.key {
      padding-left: 1px;
    }
  }
}

.buttons {
  display: flex;
  justify-content: flex-end;
  width: 100%;
}

.cant-delete-cluster-dialog {
  border-radius: var(--border-radius);
  overflow-y: scroll;
  max-height: 100vh;
  & ::-webkit-scrollbar-corner {
    background: rgba(0, 0, 0, 0);
  }
}

.capability {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
  color: var(--body-text);

  &.strike span{
    text-decoration: line-through;
  }
}

input.no-label {
  height: 54px;
}
</style>

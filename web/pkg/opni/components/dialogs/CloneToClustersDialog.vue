<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';
import { Card } from '@components/Card';
import ArrayListSelect from '@shell/components/form/ArrayListSelect';
import { exceptionToErrorsArray } from '../../utils/error';

export default {
  components: {
    Banner,
    Card,
    ArrayListSelect,
    AsyncButton,
  },
  props: {
    clusters: {
      type:     Array,
      default: () => []
    }
  },
  data() {
    return {
      errors:         '',
      clusterIds:     [],
      target:         {}
    };
  },
  methods: {
    close(cancel = true) {
      this.$modal.hide('clone-to-clusters-dialog');
      if (cancel) {
        this.$emit('cancel');
      }
    },

    open(target) {
      this.$set(this, 'target', target);
      this.$set(this, 'clusterIds', [target.clusterId]);
      this.$modal.show('clone-to-clusters-dialog');
    },

    async save(buttonDone) {
      try {
        this.$set(this, 'errors', '');
        await this.target.clone(this.clusterIds);
        this.$emit('save');
      } catch (err) {
        this.errors = exceptionToErrorsArray(err).join(';');
        buttonDone(false);
      }
    },
  },
  computed: {
    clusterOptions() {
      return this.clusters.map(c => ({
        label: c.nameDisplay,
        value: c.id
      }));
    }
  }
};
</script>

<template>
  <modal
    class="clone-to-clusters-dialog"
    name="clone-to-clusters-dialog"
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
        <h4 class="text-default-text pt-4 mb-20" v-html="`Clone <b>${target.nameDisplay}</b> to these clusters:`" />
        <div class="row">
          <div class="col span-12">
            <ArrayListSelect v-model="clusterIds" :options="clusterOptions" add-label="Add Cluster" />
          </div>
        </div>
        <div v-if="errors" class="row">
          <div class="col span-12">
            <Banner color="error">
              {{ errors }}
            </Banner>
          </div>
        </div>
      </div>
      <div slot="actions" class="buttons">
        <button class="btn role-secondary mr-10" @click="close">
          {{ t("generic.cancel") }}
        </button>

        <AsyncButton mode="edit" action-label="Clone" waiting-label="Cloning" success-label="Cloned" @click="save" />
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

.clone-to-clusters-dialog {
  border-radius: var(--border-radius);
  overflow-y: scroll;
  max-height: 100vh;
  & ::-webkit-scrollbar-corner {
    background: rgba(0, 0, 0, 0);
  }
}

::v-deep h3 {
  font-size: 14px;
}
</style>

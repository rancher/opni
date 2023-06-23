<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Card } from '@components/Card';
import { LabeledInput } from '@components/Form/LabeledInput';
import KeyValue from '@shell/components/form/KeyValue';
import { updateCluster } from '../../utils/requests/management';
import { exceptionToErrorsArray } from '../../utils/error';

export default {
  components: {
    Card,
    KeyValue,
    LabeledInput,
    AsyncButton,
  },
  data() {
    return {
      id:           '',
      name:         '',
      labels:       {},
      hiddenLabels: {},
      errors:       [],
    };
  },
  methods: {
    close() {
      this.$modal.hide('edit-cluster-dialog');
    },

    open(cluster) {
      this.$set(this, 'id', cluster.id);
      this.$set(this, 'name', cluster.name);
      this.$set(this, 'labels', cluster.visibleLabels);
      this.$set(this, 'hiddenLabels', cluster.hiddenLabels);
      this.$modal.show('edit-cluster-dialog');
    },

    async save(buttonDone) {
      try {
        await updateCluster(this.id, this.name, { ...this.hiddenLabels, ...this.labels });
        buttonDone(true);
        this.$emit('save');
        this.close();
      } catch (err) {
        this.errors = exceptionToErrorsArray(err);
        buttonDone(false);
      }
    },
  },
};
</script>

<template>
  <modal
    ref="editClusterDialog"
    class="edit-cluster-dialog"
    name="edit-cluster-dialog"
    styles="background-color: var(--nav-bg); border-radius: var(--border-radius); max-height: 90vh;"
    height="auto"
    width="600"
    :scrollable="true"
    @closed="close()"
  >
    <Card
      class="edit-cluster-card"
      :show-highlight-border="false"
      title="Edit Cluster"
    >
      <h4 ref="clusterLabel" slot="title" class="text-default-text pt-4" v-html="`Edit Cluster: ${ id }`" />
      <div slot="body" class="pt-10">
        <div class="row mb-10">
          <div class="col span-12">
            <LabeledInput v-model.trim="name" label="Name (optional)" />
          </div>
        </div>
        <div class="row">
          <KeyValue
            v-model="labels"
            mode="edit"
            :read-allowed="false"
            :value-multiline="false"
            label="Labels"
            add-label="Add Label"
          />
        </div>
      </div>

      <div slot="actions" class="buttons">
        <button class="btn role-secondary mr-10" @click="close">
          {{ t("generic.cancel") }}
        </button>

        <AsyncButton mode="edit" @click="save" />
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

.edit-cluster-dialog {
  border-radius: var(--border-radius);
  overflow-y: scroll;
  max-height: 100vh;
  & ::-webkit-scrollbar-corner {
    background: rgba(0, 0, 0, 0);
  }
}
</style>

<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Card } from '@components/Card';
import { cancelCapabilityUninstall } from '../../utils/requests/management';
import { exceptionToErrorsArray } from '../../utils/error';

export default {
  components: {
    Card,
    AsyncButton,
  },
  data() {
    return {
      errors:       [],
      capabilities: [],
      confirm:      '',
      opened:       false
    };
  },
  methods: {
    close(cancel = true) {
      this.$modal.hide('cancel-uninstall-capabilities-dialog');
      if (cancel) {
        this.$emit('cancel', this.cluster, this.capabilities);
      }
    },

    open(capabilities) {
      if (this.opened) {
        this.$set(this, 'capabilities', [...this.capabilities, ...capabilities]);
      } else {
        this.$set(this, 'opened', true);
        this.$set(this, 'capabilities', capabilities);
        this.$set(this, 'confirm', '');
        this.$modal.show('cancel-uninstall-capabilities-dialog');
      }
    },

    async save(buttonDone) {
      try {
        const cancels = this.capabilities
          .map(cap => cancelCapabilityUninstall(cap.rawCluster.id, cap.rawType));

        await Promise.all(cancels);
        this.$emit('save');
      } catch (err) {
        this.errors = exceptionToErrorsArray(err);
        buttonDone(false);
      }
    },
  },
  computed: {
    clusterName() {
      return this.cluster?.nameDisplay;
    },

    clusters() {
      return this.capabilities.map(c => c.rawCluster );
    },

    label() {
      return { metrics: 'Monitoring', logs: 'Logging' }[(this.capabilities[0] || {}).rawType];
    }
  }
};
</script>

<template>
  <modal
    class="uninstall-capabilities-dialog"
    name="cancel-uninstall-capabilities-dialog"
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
        <h4 class="text-default-text pt-4 mb-20" v-html="`Stop the uninstall of <b>${label}</b> from:`" />
        <ul>
          <li v-for="cap in capabilities" :key="cap.id">
            {{ cap.clusterNameDisplay }}
          </li>
        </ul>
      </div>
      <div slot="actions" class="buttons">
        <button class="btn role-secondary mr-10" @click="close">
          {{ t("generic.cancel") }}
        </button>

        <AsyncButton mode="edit" action-label="Stop" waiting-label="Stopping" success-label="Cancelled" @click="save" />
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

.uninstall-capabilities-dialog {
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

::v-deep h3 {
  font-size: 14px;
}
</style>

<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Card } from '@components/Card';
import RadioGroup from '@components/Form/Radio/RadioGroup';
import { Banner } from '@components/Banner';
import { exceptionToErrorsArray } from '@pkg/opni/utils/error';

export default {
  components: {
    Banner,
    Card,
    AsyncButton,
    RadioGroup
  },
  data() {
    return {
      errors:       [],
      capabilities: [],
      deleteData:   false,
      confirm:      '',
      opened:         false,
    };
  },
  methods: {
    close(cancel = true) {
      this.$set(this, 'opened', false);
      this.$modal.hide('uninstall-capabilities-dialog');
      if (cancel) {
        this.$emit('cancel', this.cluster, this.capabilities);
      }
    },

    open(capabilities) {
      if (this.opened) {
        this.$set(this, 'capabilities', [...this.capabilities, ...capabilities]);
      } else {
        this.$set(this, 'opened', true);
        this.$set(this, 'deleteData', false);
        this.$set(this, 'capabilities', capabilities);
        this.$set(this, 'confirm', '');
        this.$modal.show('uninstall-capabilities-dialog');
      }
    },

    async save(buttonDone) {
      try {
        const uninstalls = this.capabilities
          .filter(cap => cap.isInstalled)
          .map(cap => cap.uninstall(this.deleteData));

        await Promise.all(uninstalls);
        this.$emit('save');
      } catch (err) {
        this.errors = exceptionToErrorsArray(err);
        buttonDone(false);
      }
    },
  },
  computed: {
    label() {
      return (this.capabilities[0] || {}).nameDisplay;
    },

    showDeleteData() {
      return (this.capabilities[0] || {}).type !== 'alerting';
    }
  }
};
</script>

<template>
  <modal
    ref="editClusterDialog"
    class="uninstall-capabilities-dialog"
    name="uninstall-capabilities-dialog"
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
        <h4 class="text-default-text pt-4 mb-20" v-html="`Uninstall <b>${label}</b> from the following clusters:`" />
        <ul>
          <li v-for="cap in capabilities" :key="cap.id">
            {{ cap.clusterNameDisplay }}
          </li>
        </ul>
        <div v-if="showDeleteData" class="row mt-10">
          <div class="col span-12">
            <RadioGroup
              v-model="deleteData"
              name="deletedData"
              label="Do you also want to delete the data?"
              :labels="['Yes', 'No']"
              :options="[true, false]"
            />
          </div>
        </div>
        <div class="row">
          <div class="col span-12">
            <Banner color="warning">
              <div>
                <span>Uninstalling capabilities will permanently remove them. Are you sure you want to uninstall</span> <b>{{ label }}</b>?
              </div>
            </Banner>
            To confirm uninstall enter <b>{{ label }}</b> below:
            <input v-model="confirm" class="no-label mt-5" type="text" />
          </div>
        </div>
      </div>
      <div slot="actions" class="buttons">
        <button class="btn role-secondary mr-10" @click="close">
          {{ t("generic.cancel") }}
        </button>

        <AsyncButton mode="edit" :disabled="label !== confirm" @click="save" />
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

    hr {
      display: none;
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

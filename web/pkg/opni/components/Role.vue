<script>
import { mapGetters } from 'vuex';
import { LabeledInput } from '@components/Form/LabeledInput';
import AsyncButton from '@shell/components/AsyncButton';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { Banner } from '@components/Banner';
import ArrayListSelect from '@shell/components/form/ArrayListSelect';
import KeyValue from '@shell/components/form/KeyValue';
import MatchExpressions from '@shell/components/form/MatchExpressions';
import { createRole } from '@pkg/opni/utils/requests/management';
import { exceptionToErrorsArray } from '../utils/error';

export default {
  components: {
    ArrayListSelect,
    AsyncButton,
    KeyValue,
    LabeledInput,
    MatchExpressions,
    Tab,
    Tabbed,
    Banner,
  },

  data() {
    return {
      name:             '',
      roleName:         '',
      subjects:         [],
      taints:           [],
      clusterIds:       [],
      matchLabels:      {},
      matchExpressions: [],
      error:            '',
    };
  },

  methods: {
    async save(buttonCallback) {
      if (this.name === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }
      try {
        await createRole(this.name, this.clusterIds, this.matchLabelsToSave);
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);

        return;
      }
      this.$set(this, 'error', '');
      buttonCallback(true);
      this.$router.replace({ name: 'roles' });
    },

    cancel() {
      this.$router.replace({ name: 'roles' });
    }
  },

  computed: {
    ...mapGetters({ clusters: 'opni/clusters' }),

    matchLabelsToSave() {
      return {
        matchLabels:      this.matchLabels,
        matchExpressions: this.matchExpressions,
      };
    },

    clusterIdOptions() {
      return this.clusters.map(cluster => ({
        label: cluster.nameDisplay,
        value: cluster.id,
      }));
    }
  },
};
</script>
<template>
  <div>
    <div class="row mb-20">
      <div class="col span-12">
        <LabeledInput
          v-model="name"
          label="Name"
          :required="true"
        />
      </div>
    </div>
    <Tabbed :side-tabs="true" class="mb-20">
      <Tab
        name="clusters"
        c
        :label="t('opni.monitoring.role.tabs.clusters.label')"
        :weight="3"
      >
        <ArrayListSelect
          v-model="clusterIds"
          :options="clusterIdOptions"
          :array-list-props="{
            addLabel: t('opni.monitoring.role.tabs.clusters.add'),
          }"
        />
      </Tab>
      <Tab
        name="matchLabels"
        :label="t('opni.monitoring.role.tabs.matchLabels.label')"
        :weight="2"
      >
        <KeyValue
          v-model="matchLabels"
          mode="edit"
          :read-allowed="false"
          :value-multiline="false"
          label="Labels"
          add-label="Add Label"
        />
      </Tab>
      <Tab
        name="matchExpressions"
        :label="t('opni.monitoring.role.tabs.matchExpressions.label')"
        :weight="1"
      >
        <MatchExpressions
          v-model="matchExpressions"
          :initial-empty-row="false"
          type="pod"
        />
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn btn-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <AsyncButton mode="edit" @click="save" />
    </div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
  </div>
</template>

<style lang="scss" scoped>
.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}

.install-command {
  width: 100%;
}

::v-deep .warning {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}
</style>

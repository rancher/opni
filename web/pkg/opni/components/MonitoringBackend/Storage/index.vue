<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import { Storage } from '@pkg/opni/api/opni';
import { createComputedDuration } from '@pkg/opni/utils/computed';
import S3 from './S3';
import Azure from './Azure';
import Gcs from './Gcs';
import Swift from './Swift';

export const SECONDS_IN_DAY = 86400;

export default {
  components: {
    Azure, Gcs, S3, UnitInput, LabeledSelect, Swift
  },

  props: {
    value: {
      type:     Object,
      required: true
    },
  },

  data() {
    return { Storage };
  },

  computed: {
    storageOptions() {
      return [
        { label: 'Filesystem', value: 'filesystem' },
        { label: 'S3', value: 's3' },
        { label: 'GCS', value: 'gcs' },
        { label: 'Azure', value: 'azure' },
        { label: 'Swift', value: 'swift' },
      ];
    },

    retentionPeriod: createComputedDuration('value.cortexConfig.limits.compactorBlocksRetentionPeriod', SECONDS_IN_DAY),
  },

  methods: {},
  watch:   {
    storageOptions() {
      const values = this.storageOptions.map(so => so.value);

      if (!values.includes(this.value.cortexConfig.storage.backend)) {
        this.$set(this.value.cortexConfig.storage, 'backend', values[0]);
      }
    }
  },
};
</script>
<template>
  <div class="m-0">
    <div>
      <div class="row" :class="{ bottom: value.cortexConfig.storage.backend !== 'filesystem' }">
        <div class="col span-6">
          <LabeledSelect v-model="value.cortexConfig.storage.backend" :options="storageOptions" label="Storage Type" />
        </div>
        <div class="col span-6">
          <UnitInput
            v-model="retentionPeriod"
            class="retention-period"
            label="Data Retention Period"
            suffix="days"
            tooltip="A value of 0 will retain data indefinitely"
          />
        </div>
      </div>
      <S3 v-if="value.cortexConfig.storage.backend === 's3'" v-model="value" class="mt-15" />
      <Azure v-if="value.cortexConfig.storage.backend === 'azure'" v-model="value" class="mt-15" />
      <Gcs v-if="value.cortexConfig.storage.backend === 'gcs'" v-model="value" class="mt-15" />
      <Swift v-if="value.cortexConfig.storage.backend === 'swift'" v-model="value" class="mt-15" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
header {
  width: 100%;
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0;
}

::v-deep {
  .not-enabled {
    text-align: center;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    height: 100%;
  }

  .enabled {
    width: 100%;
  }
}
</style>

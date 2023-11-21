<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import { Checkbox } from '@components/Form/Checkbox';
import UnitInput from '@shell/components/form/UnitInput';
import { createComputedDuration } from '@pkg/opni/utils/computed';

export default {
  components: {
    Checkbox, UnitInput, LabeledInput
  },

  props: {
    value: {
      type:     Object,
      required: true
    },
  },

  created() {
  },

  data() {
    return {};
  },

  computed: {
    idleConnTimeout:       createComputedDuration('value.cortexConfig.storage.azure.http.idleConnTimeout'),
    responseHeaderTimeout: createComputedDuration('value.cortexConfig.storage.azure.http.responseHeaderTimeout'),
    tlsHandshakeTimeout:   createComputedDuration('value.cortexConfig.storage.azure.http.tlsHandshakeTimeout'),
    expectContinueTimeout: createComputedDuration('value.cortexConfig.storage.azure.http.expectContinueTimeout'),
  },

  methods: {},
  watch:   {},
};
</script>
<template>
  <div class="m-0">
    <div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.gcs.bucketName" label="Bucket Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.gcs.serviceAccount" label="Service Account" type="password" />
        </div>
      </div>
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

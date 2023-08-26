<script>
import { LabeledInput } from '@components/Form/LabeledInput';

export default {
  components: { LabeledInput },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },
};
</script>
<template>
  <div class="row">
    <div class="col span-12">
      <LabeledInput v-model="value.integrationKey" label="Integration Key" :required="true" />
    </div>
  </div>
</template>

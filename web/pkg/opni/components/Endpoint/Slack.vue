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
  <div class="row mt-20">
    <div class="col span-6">
      <LabeledInput v-model="value.webhookUrl" label="Webhook URL" :required="true" />
    </div>
    <div class="col span-6">
      <LabeledInput v-model="value.channel" label="Channel" :required="true" />
    </div>
  </div>
</template>

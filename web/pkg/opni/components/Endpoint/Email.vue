<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import { Checkbox } from '@components/Form/Checkbox';

export default {
  components: { Checkbox, LabeledInput },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },
};
</script>
<template>
  <div>
    <div class="row mt-20 bottom mb-10">
      <div class="col span-6">
        <LabeledInput v-model="value.to" label="To Email" :required="true" />
      </div>
      <div class="col span-6">
        <LabeledInput v-model="value.smtpFrom" label="From Email" :required="true" />
      </div>
    </div>
    <h4>SMTP</h4>
    <div class="row mt-10">
      <div class="col span-6">
        <LabeledInput v-model="value.smtpSmartHost" label="Smart Host" :required="true" />
      </div>
      <div class="col span-6">
        <LabeledInput v-model="value.smtpAuthIdentity" label="Identity" :required="true" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-6">
        <LabeledInput v-model="value.smtpAuthUsername" label="Username" :required="true" />
      </div>
      <div class="col span-6">
        <LabeledInput v-model="value.smtpAuthPassword" label="Password" type="password" :required="true" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-6">
        <Checkbox v-model="value.smtpRequireTLS" label="Required TLS" />
      </div>
    </div>
  </div>
</template>

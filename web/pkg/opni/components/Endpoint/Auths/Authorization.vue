<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';

export default {
  components: { LabeledInput, LabeledSelect },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  data() {
    const credentialTypes = [
      {
        label: 'Credentials',
        value: 'credentials'
      },
      {
        label: 'Credentials File',
        value: 'credentialsFile'
      }
    ];

    const credentialType = credentialTypes.map(pt => pt.value).find(type => this.value[type]) || credentialTypes[0].value;

    if (!this.value.type) {
      this.$set(this.value, 'type', 'Bearer');
    }

    return {
      credentialTypes,
      credentialType
    };
  },
  watch: {
    credentialType() {
      this.$set(this.value, 'credentials', null);
      this.$set(this.value, 'credentialsFile', null);
    }
  }
};
</script>
<template>
  <div>
    <div class="row">
      <div class="col span-12">
        <LabeledInput v-model="value.type" label="Authentication Type" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-4">
        <LabeledSelect v-model="credentialType" :options="credentialTypes" label="Credential Type" />
      </div>
      <div class="col span-8">
        <LabeledInput v-if="credentialType === 'credentials'" v-model="value.credentials" label="Credentials" type="password" />
        <LabeledInput v-else v-model="value.credentialsFile" label="Credentials File" />
      </div>
    </div>
  </div>
</template>

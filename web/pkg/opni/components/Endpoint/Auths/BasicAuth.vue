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
    const passwordTypes = [
      {
        label: 'Password',
        value: 'password'
      },
      {
        label: 'Password File',
        value: 'passwordFile'
      }
    ];

    const passwordType = passwordTypes.map(pt => pt.value).find(type => this.value[type]) || passwordTypes[0].value;

    return {
      passwordTypes,
      passwordType
    };
  },
  watch: {
    passwordType() {
      this.$set(this.value, 'password', null);
      this.$set(this.value, 'passwordFile', null);
    }
  }
};
</script>
<template>
  <div>
    <div class="row mb-10">
      <div class="col span-12">
        <LabeledInput v-model="value.username" label="Username" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-4">
        <LabeledSelect v-model="passwordType" :options="passwordTypes" label="Password Type" />
      </div>
      <div class="col span-8">
        <LabeledInput v-if="passwordType === 'password'" v-model="value.password" label="Password" type="password" />
        <LabeledInput v-else v-model="value.passwordFile" label="Password File" :password="true" />
      </div>
    </div>
  </div>
</template>

<script>
import UnitInput from '@shell/components/form/UnitInput';
import EnabledDisabled from '../EnabledDisabled';
import Resources from './Resources';

export default {
  components: {
    EnabledDisabled, Resources, UnitInput
  },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  created() {
    if (!this.value.resources) {
      this.$set(this.value, 'resources', { limits: {}, requests: {} });
    }
  }
};
</script>
<template>
  <EnabledDisabled
    v-model="value.enabled"
    disabled-message="The Dashboard is not currently enabled. Enabling it will install additional resources."
  >
    <div class="row border-bottom mb-10">
      <div class="col span-6">
        <UnitInput v-model="value.replicas" label="Replicas" :suffix="false" />
      </div>
    </div>
    <Resources v-model="value.resources" :use-resource-requirements="true" />
  </EnabledDisabled>
</template>

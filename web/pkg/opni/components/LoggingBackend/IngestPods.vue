<script>
import UnitInput from '@shell/components/form/UnitInput';
import { Checkbox } from '@components/Form/Checkbox';
import EnabledDisabled from '../EnabledDisabled';
import Resources from './Resources';
import Advanced from './Advanced';

export default {
  components: {
    Checkbox,
    Resources,
    UnitInput,
    Advanced,
    EnabledDisabled
  },

  props: {
    value: {
      type:    Object,
      default: () => {},
    },
  },

  data() {
    return {};
  },
};
</script>
<template>
  <EnabledDisabled v-model="value.enabled" disabled-message="Would you like to enable the separation of Ingest Pods?" disable-label="Disable pod separation">
    <div class="row border-bottom mb-10">
      <div class="col span-6">
        <UnitInput v-model="value.replicas" label="Replicas" :suffix="false" />
      </div>
      <div class="col span-6 middle">
        <Checkbox v-model="value.enableAntiAffinity" label="Spread across nodes" />
      </div>
    </div>
    <Resources v-model="value" class="mb-10 border-bottom" />
    <Advanced v-model="value" />
  </EnabledDisabled>
</template>

<style lang="scss" scoped></style>

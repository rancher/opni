<script>
import UnitInput from '@shell/components/form/UnitInput';
import { Checkbox } from '@components/Form/Checkbox';
import Resources from './Resources';
import Storage from './Storage';
import Advanced from './Advanced';

export default {
  components: {
    Checkbox,
    Resources,
    Storage,
    UnitInput,
    Advanced
  },

  props: {
    value: {
      type:    Object,
      default: () => {},
    },

    storageClassOptions: {
      type:     Array,
      required: true
    }
  },

  data() {
    return { showAdvanced: false };
  },

  methods: {
    toggleAdvanced() {
      this.$set(this, 'showAdvanced', !this.showAdvanced);
    }
  },
};
</script>
<template>
  <div>
    <div class="row border-bottom mb-10">
      <div class="col span-6">
        <UnitInput v-model="value.replicas" label="Replicas" base-unit="" />
      </div>
      <div class="col span-6 middle">
        <Checkbox v-model="value.enableAntiAffinity" label="Spread across nodes" />
      </div>
    </div>
    <Storage v-model="value" class="border-bottom mb-10" :storage-class-options="storageClassOptions" />
    <Resources v-model="value" class="border-bottom mb-10" />
    <Advanced v-model="value" />
  </div>
</template>

<style lang="scss" scoped></style>

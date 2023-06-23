<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';
import EnabledDisabled from '../EnabledDisabled';
import Storage from './Storage';
import Advanced from './Advanced';

export default {
  components: {
    EnabledDisabled,
    LabeledSelect,
    Storage,
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
    return {};
  },

};
</script>
<template>
  <EnabledDisabled v-model="value.enabled" disabled-message="Would you like to enable the separation of Controlplane Pods?" disable-label="Disable pod separation">
    <div class="row border-bottom mb-10">
      <div class="col span-6">
        <LabeledSelect v-model="value.replicas" label="Replicas" :options="['1', '3', '5']" />
      </div>
    </div>
    <Storage v-model="value" :storage-class-options="storageClassOptions" :has-disk-size="false" class="mb-10 border-bottom" />
    <Advanced v-model="value" />
  </EnabledDisabled>
</template>

<style lang="scss" scoped>
</style>

<script>
import Tolerations from '@shell/components/form/Tolerations';
import KeyValue from '@shell/components/form/KeyValue';

export default {
  components: {
    Tolerations,
    KeyValue
  },

  props: {
    value: {
      type:     Object,
      required: true
    },
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
    <a class="hand block mb-10" @click="toggleAdvanced">{{ showAdvanced ? 'Hide Advanced' : 'Show Advanced' }}</a>
    <div v-if="showAdvanced">
      <Tolerations v-model="value.tolerations" class="mb-10" />
      <KeyValue v-model="value.nodeSelector" add-label="Add Node Selector" :read-allowed="false" />
    </div>
  </div>
</template>

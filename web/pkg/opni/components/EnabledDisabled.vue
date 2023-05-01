<script>
export default {
  props: {
    value: {
      type:     Boolean,
      required: true
    },

    disabledMessage: {
      type:     String,
      required: true
    },

    disableLabel: {
      type:    String,
      default: 'Disable'
    }
  },

  methods: {
    enable() {
      this.$emit('input', true);
    },
    disable() {
      this.$emit('input', false);
    }
  }
};
</script>
<template>
  <div>
    <button v-if="value" class="btn role-secondary disable-button" @click="disable">
      {{ disableLabel }}
    </button>
    <div v-if="value">
      <slot />
    </div>
    <div v-else slot="body" class="not-enabled">
      <h5>{{ disabledMessage }}</h5>
      <button class="btn role-primary" @click="enable">
        Enable
      </button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.disable-button {
  position: absolute;
  right: 20px;
  top: 20px;
}
</style>

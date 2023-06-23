<script>

export default {
  components: {},

  props: {
    label: {
      type:     String,
      required: true
    },
    type: {
      type:     String,
      required: true
    },
    cluster: {
      type:     Object,
      required: true
    },
    isBackendInstalled: {
      type:     Boolean,
      required: true
    }
  },

  computed: {
    dynamicClass() {
      const status = this.cluster.capabilityStatus[this.type] || {};
      const state = this.state(status);

      return {
        [state]:     true,
        [this.type]: true,
        loading:     status.pending
      };
    },
    tooltip() {
      const status = this.cluster.capabilityStatus[this.type];

      if (status?.message) {
        return status.message;
      }

      if (this.cluster.isCapabilityInstalled(this.type)) {
        return 'Click to uninstall';
      }

      if (!this.isBackendInstalled) {
        return `You need to install the backend before installing the capability.`;
      }

      return 'Click to install';
    }
  },

  methods: {
    click(ev) {
      ev.stopPropagation();
      this.cluster.toggleCapability(this.type);
    },

    state(status) {
      if (status.state) {
        return status.state;
      }

      if (this.cluster.isCapabilityInstalled(this.type)) {
        return '';
      }

      if (!this.isBackendInstalled) {
        return 'not-allowed';
      }

      return 'disabled';
    }
  }
};
</script>
<template>
  <span v-tooltip="tooltip" class="bubble capability mr-5 metrics" :class="dynamicClass" @click="click"><span class="icon icon-spinner icon-spin"></span>{{ label }}</span>
</template>

<style lang="scss" scoped>
.capability {
    position: relative;
    cursor: pointer;
    padding: 3px 8px;

    &.not-allowed {
      cursor: not-allowed;
      border-color: var(--disabled-bg);
      color: var(--disabled-bg);
    }

    &:not(.loading) .icon {
        display: none;
    }
}

.icon {
    margin-right: 3px;
}
</style>

<script>
export default {
  props: {
    value: {
      type:    [String, Number],
      default: null,
    },

    row: {
      type:    Object,
      default: null,
    },

    getClass: {
      type:    Function,
      default: () => ''
    },

    onClick: {
      type:    Function,
      default: null
    }
  },
  computed: {
    computedClass() {
      return this.getClass(this.row, this.value);
    }
  },

  methods: {
    click() {
      if (this.onClick) {
        return this.onClick(this.row, this.value);
      }
    }
  }
};
</script>

<template>
  <span :class="computedClass" @click="click">{{ value }}</span>
</template>

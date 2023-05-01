<script>
export default {
  props: {
    value: {
      type:     String,
      default: '',
    },
    iconSize: {
      type:    String,
      default: null,
    },
  },
};
</script>

<template>
  <span><i v-if="value" :class="{'icon': true, [value]: true, [iconSize]: true}" /></span>
</template>

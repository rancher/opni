<script>
export default {
  props: {
    simple: {
      type:    Boolean,
      default: false
    }
  },
};
</script>

<template>
  <header :class="{'simple': simple}">
    <slot />
  </header>
</template>
<style lang="scss" scoped>
  header {
    display: grid;
    align-items: center;
    padding-left: 20px;

    background-color: var(--header-bg);
    border-bottom: var(--header-border-size) solid var(--header-border);

    .title {
      border-left: 1px solid var(--header-border);
      padding-left: 10px;
      opacity: 0.7;
      text-transform: uppercase;
    }
  }
</style>

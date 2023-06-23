<script>
export default {
  props: {
    value: {
      type:     Array,
      default: () => []
    }
  },
};
</script>

<template>
  <div>
    <span v-for="el in value" :key="el" class="bubble mr-5">
      {{ el }}
    </span>
  </div>
</template>

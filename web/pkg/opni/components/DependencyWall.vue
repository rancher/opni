<script>
export default {
  props: {
    title: {
      type:     String,
      required: true
    },
    dependencyName: {
      type:     String,
      required: true
    },
    routeName: {
      type:     String,
      required: true
    }
  },
};
</script>
<template>
  <div class="main">
    <h4>
      {{ dependencyName }} must be enabled to use {{ title }}. <n-link :to="{name: routeName}">
        Click here
      </n-link> to enable {{ dependencyName }}.
    </h4>
  </div>
</template>

<style lang="scss" scoped>
.main {
  text-align: center;
}
</style>

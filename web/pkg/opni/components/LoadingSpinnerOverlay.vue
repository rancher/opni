<script>
import LoadingSpinner from '@pkg/opni/components/LoadingSpinner';

export default {
  components: { LoadingSpinner },

  props: {
    loading: {
      type:     Boolean,
      required: true
    }
  }
};
</script>
<template>
  <div class="loading-spinner-overlay">
    <slot />
    <div v-if="loading" class="loading-spinner-container">
      <LoadingSpinner />
    </div>
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .initial-load-spinner {
      border-color: rgb(219, 136, 240);
      border-top-color: var(--primary);
    }
}

.loading-spinner-overlay {
    position: relative;
}
.loading-spinner-container {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;

  display: flex;
  justify-content: center;
  align-items: center;

  background-color: rgba(242, 242, 242, 0.6);
}
</style>

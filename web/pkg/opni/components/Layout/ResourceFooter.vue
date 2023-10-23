<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';

export default {
  components: {
    AsyncButton,
    Banner,
  },

  props: {
    error: {
      type:    String,
      default: null
    },

    cancel: {
      type:    Function,
      default: null
    },

    save: {
      type:    Function,
      default: null
    }
  }
};
</script>

<template>
  <div>
    <div class="resource-footer">
      <slot name="before" />
      <button v-if="cancel" class="btn btn-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <slot name="between" />
      <AsyncButton v-if="save" mode="edit" @click="save" />
      <slot name="after" />
    </div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
  </div>
</template>

<style lang="scss" scoped>
.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}
</style>

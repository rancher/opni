<script>
import { BadgeState } from '@components/BadgeState';

export default {
  components: { BadgeState },
  props:      {
    value: {
      type:     Object,
      required: true
    },
  },

  computed: {
    color() {
      return `bg-${ this.value.state }`;
    }
  }
};
</script>

<template>
  <div>
    <BadgeState :color="color" :label="value.shortMessage || value.message" />
  </div>
</template>

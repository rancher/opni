<script>
import SideNavColumnItem from './Item';

function findLastIndex(arr, fn) {
  for (let i = arr.length - 1; i >= 0; i--) {
    if (fn( arr[i], i, arr )) {
      return i;
    }
  }

  return null;
}

export default {
  components: { SideNavColumnItem },
  props:      {
    items: {
      type:     Array,
      required: true
    }
  },

  data() {
    return { selectedIndex: findLastIndex(this.items, i => this.$router.history.current.path.includes(i.route)) };
  },

  watch: {
    $route() {
      this.$set(this, 'selectedIndex', findLastIndex(this.items, i => this.$router.history.current.path.includes(i.route)));
    },

    items() {
      this.$set(this, 'selectedIndex', findLastIndex(this.items, i => this.$router.history.current.path.includes(i.route)));
    }
  },
};
</script>

<template>
  <div class="items">
    <SideNavColumnItem v-for="(item, i) in items" :key="i" :item="item" :selected="selectedIndex === i" />
  </div>
</template>
<style lang="scss" scoped>
  .items {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
</style>

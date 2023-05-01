<script>
export default {
  components: { },

  props: {
    open: {
      type:    Boolean,
      default: true
    },
  },

  data() {
    return { maximized: false };
  },

  methods: {
    close() {
      this.$emit('close');
      this.$set(this, 'maximized', false);
    },

    toggle() {
      this.$set(this, 'maximized', !this.maximized);
    }
  },

  computed: {
    toggleClass() {
      return this.maximized ? 'icon-chevron-down' : 'icon-chevron-up';
    }
  }
};
</script>

<template>
  <div class="drawer p-10 pb-0" :class="{ maximized }" @click.stop>
    <div class="title-bar">
      <div class="action">
        <button @click="toggle">
          {{ maximized ? "Hide Details" : "Show Details" }}
        </button>
      </div>
      <slot name="title">
      </slot>
    </div>
    <div v-if="maximized" class="content">
      <slot />
    </div>
  </div>
</template>

<style lang="scss">
  .drawer {
    display: flex;
    flex-direction: column;

    position: absolute;
    left: 230px;
    right: 0;
    bottom: 0;
    height: 56px;
    z-index: 11;

    border-radius: var(--border-radius);
    box-shadow: 0 0 20px var(--shadow);
    background-color: #FFF;
    transition: height 0.2s;

    &.maximized {
      height: 100%;
    }

    .title-bar {
        display: flex;
        flex-direction: row;
        justify-content: space-between;
        align-items: center;

        .action {
          display: flex;
          flex-direction: row;

          button {
            line-height: initial;
            min-height: 30px;
            padding: 0px 20px;
            transform: perspective(10px) rotateX(-1deg);

            &:focus {
                box-shadow: none;
                background-color: #e9e9ed;
            }

          }
        }
    }

    .action {
        position: absolute;
        left: 0;
        right: 0;
        top: 0;
        bottom: 0;

        display: flex;
        flex-direction: row;
        justify-content: center;
        align-items: flex-start;
    }

    .content {
      position: relative;
      height: 100%;
    }
  }
</style>

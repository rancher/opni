<script>
export default {
  props: {
    value: {
      type:     String,
      default: ''
    },

    prefix: {
      type:    String,
      default: ''
    },

    suffix: {
      type:    String,
      default: ''
    },
  },
  data() {
    return { copied: false };
  },
  computed: {
    isToken() {
      // Checks if the displayed value is a token string or a friendly name.
      // Tokens are 64-character hex strings with a '.' separating the first
      // 12 and last 52 characters.
      return /^[0-9a-f]{12}\.[0-9a-f]{52}$/i.test(this.value);
    },
    tokenId() {
      return this.value.split('.')[0];
    },
    tokenSecret() {
      return this.value.split('.')[1];
    },
  },
  methods: {
    copyToken(ev) {
      ev.stopPropagation();
      ev.preventDefault();

      this.$copyText(this.value).then(() => {
        this.copied = true;
        setTimeout(() => {
          this.copied = false;
        }, 2000);
      });
    }
  }
};
</script>

<template>
  <span
    v-if="isToken"
    v-tooltip="{'content': copied ? 'Copied!' : 'Copy Token', hideOnTargetClick: false}"
    class="token"
    @click="copyToken"
  >
    <span class="token-id">
      {{ tokenId }}
    </span>
    <span class="token-dot">
      .
    </span>
    <span class="token-secret">
      {{ tokenSecret }}
    </span>
  </span>
  <span v-else>
    {{ value }}
  </span>
</template>

<style lang="scss" scoped>
.token {
  display: inline-flex;
  flex-grow: 0;
  flex-shrink: 0;
  flex-basis: auto;
  cursor: pointer;
}
.token-id {
  color: var(--primary);
  font-family: $mono-font;
}
.token-dot {
  font-family: $mono-font;
}
.token-secret {
  color: var(--app-other-accent);
  font-family: $mono-font;
}
</style>

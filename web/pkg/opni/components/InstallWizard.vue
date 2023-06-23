<script>
export default {
  name: 'InstallMenu',

  props: {
    getStartedLink: {
      type:    Object,
      default: null,
    },

    initStepIndex: {
      type:    Number,
      default: 0
    },

    steps: {
      type:    Array,
      default: null,
    },
  },

  data() {
    return { activeStep: null };
  },

  created() {
    this.activeStep = this.steps[this.initStepIndex];
  },

  computed: {
    activeStepIndex() {
      return this.steps.findIndex(s => s.name === this.activeStep.name);
    }
  },

  methods: {
    goToStep(number, fromNav) {
      if (number < 1) {
        return;
      }

      if (number === 1 && fromNav) {
        return;
      }

      const selected = this.steps[number - 1];

      if (!selected || (!this.isAvailable(selected) && number !== 1)) {
        return;
      }

      this.activeStep = selected;

      this.$emit('next', { step: selected });
    },

    isAvailable(step) {
      if (!step) {
        return false;
      }

      const idx = this.steps.findIndex(s => s.name === step.name);

      if (idx === 0) {
        return false;
      }

      for (let i = 0; i < idx; i++) {
        if (this.steps[i].ready === false) {
          return false;
        }
      }

      return true;
    },

    next() {
      this.goToStep(this.activeStepIndex + 2);
    },
  },
};
</script>

<template>
  <div>
    <div class="header mt-20 mb-20">
      <div class="title">
        <div class="product-image">
          <!-- <img src="../../assets/icon-kubewarden.svg" class="logo" /> -->
        </div>
        <div class="subtitle mr-20">
          <h2>
            {{ t('kubewarden.title') }}
          </h2>
          <span class="subtext">{{ t('kubewarden.dashboard.install') }}</span>
        </div>
        <div class="subtitle">
          <h2>{{ t('wizard.step', {number: activeStepIndex + 1}) }}</h2>
          <slot name="bannerSubtext">
            <span class="subtext">{{ activeStep.label }}</span>
          </slot>
        </div>
      </div>

      <div class="step-sequence">
        <ul class="steps" tabindex="0">
          <template v-for="(step, idx) in steps">
            <li
              :id="step.name"
              :key="step.name + 'li'"
              :class="{
                step: true,
                active: step.name === activeStep.name,
                disabled: !isAvailable(step),
              }"
              role="presentation"
            >
              <span
                :aria-controls="'step' + idx + 1"
                :aria-selected="step.name === activeStep.name"
                role="tab"
                class="controls"
                @click.prevent="goToStep(idx + 1, true)"
              >
                <span
                  class="icon icon-lg"
                  :class="{
                    'icon-dot': step.name === activeStep.name,
                    'icon-dot-open': step.name !== activeStep.name,
                  }"
                />
                <span>
                  {{ step.label }}
                </span>
              </span>
            </li>
            <div
              v-if="idx !== steps.length - 1"
              :key="step.name"
              class="divider"
            />
          </template>
        </ul>
      </div>
    </div>

    <slot name="stepContainer mt-20" :activeStep="activeStep">
      <template v-for="step in steps">
        <div
          v-if="step.name === activeStep.name || step.hidden"
          :key="step.name"
          class="step-container"
          :class="{ hide: step.name !== activeStep.name && step.hidden }"
        >
          <slot :step="step" :name="step.name" />
        </div>
      </template>
    </slot>
  </div>
</template>

<style lang="scss" scoped>
.header {
  display: flex;

  & .title {
    display: flex;
    flex-basis: 40%;
    align-items: center;

    & .product-image {
      min-width: 50px;
      height: 50px;
      margin: 10px 10px 10px 0;
      overflow: hidden;
      .logo {
        min-width: 50px;
        height: 50px;
      }
    }
  }
}

.step-sequence {
  flex: 1;
  min-height: 60px;
  display: flex;
  width: 100%;

  .steps {
    flex: 1;
    margin: 0 30px;
    display: flex;
    justify-content: space-between;
    list-style-type: none;
    padding: 0;

    &:focus {
      outline: none;
      box-shadow: none;
    }

    & li.step {
      display: flex;
      flex-direction: row;
      flex-grow: 1;
      align-items: center;

      & > span > span:last-of-type {
        padding-bottom: 0;
      }

      &:last-of-type {
        flex-grow: 0;
      }

      & .controls {
        display: flex;
        flex-direction: column;
        align-items: center;
        width: 40px;
        overflow: visible;
        padding-top: 15px;

        & > span {
          padding-bottom: 5px;
          margin-bottom: 5px;
          white-space: nowrap;
        }
      }

      &.active .controls {
        color: var(--primary);
      }

      &:not(.disabled) {
        & .controls:hover > * {
          color: var(--primary) !important;
          cursor: pointer;
        }
      }

      &:not(.active) {
        & .controls > * {
          color: var(--input-disabled-text);
          text-decoration: none;
        }
      }
    }

    & .divider {
      flex-basis: 100%;
      border-top: 1px solid var(--border);
      position: relative;
      top: 25px;
    }
  }
}

.step-container {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
}
</style>

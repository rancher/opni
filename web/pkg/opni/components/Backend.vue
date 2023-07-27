<script>
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import { Banner } from '@components/Banner';
import LoadingSpinner from '@pkg/opni/components/LoadingSpinner';
import { exceptionToErrorsArray } from '../utils/error';

export default {
  components: {
    AsyncButton,
    Banner,
    Loading,
    LoadingSpinner
  },

  props: {
    title: {
      type:     String,
      required: true
    },

    showDetail: {
      type:    Boolean,
      default: true
    },

    isEnabled: {
      type:     Function,
      required: true
    },

    isUpgradeAvailable: {
      type:    Function,
      default: () => false
    },

    upgrade: {
      type:    Function,
      default: () => {}
    },

    getStatus: {
      type:    Function,
      default: () => null
    },

    getConfig: {
      type:    Function,
      default: () => null
    },

    save: {
      type:     Function,
      required: true
    },

    disable: {
      type:     Function,
      required: true
    },

    capabilityType: {
      type:    String,
      default: null
    }
  },

  async fetch() {
    await this.load();
  },

  created() {
    this.$set(this, 'statusInterval', setInterval(this.loadStatus, 5000));
  },

  beforeDestroy() {
    clearInterval(this.statusInterval);
  },

  data() {
    return {
      editing:          false,
      enabled:          false,
      error:            '',
      upgradeable:      false,
      status:           null,

      statsInterval:    null,
      statusInterval:   null,
      loadingConfig:    false,
    };
  },

  methods: {
    async tryFn(fn, finished, always) {
      try {
        const val = await fn();

        if (finished) {
          finished(true);
        }

        return val;
      } catch (ex) {
        if (finished) {
          finished(false);
        }
        this.$set(this, 'error', exceptionToErrorsArray(ex).join(';'));
      } finally {
        if (always) {
          always();
        }
      }
    },

    enable() {
      this.$set(this, 'status', null);
      this.$set(this, 'editing', true);
      this.load();
    },

    editFn() {
      this.$set(this, 'editing', true);
    },

    loadStatus() {
      if (this.enabled || this.editing) {
        this.getStatus().then((status) => {
          this.$set(this, 'status', status);
        });
      }
    },

    async saveFn(cb) {
      await this.tryFn(async() => {
        this.$set(this, 'error', '');
        await this.save();
        this.$set(this, 'editing', false);
        this.$set(this, 'enabled', true);
        await this.loadStatus();

        if (document.querySelector('main')) {
          document.querySelector('main').scrollTop = 0;
          window.scrollTo({ top: 0 });
        }
      }, cb);
    },

    async disableFn() {
      await this.tryFn(async() => {
        await this.disable();
        this.$set(this, 'editing', false);
        this.$set(this, 'enabled', false);
        await this.loadStatus();
      }, undefined);
    },

    cancel() {
      this.$set(this, 'editing', false);
      this.$set(this, 'error', '');
    },

    async load() {
      const isEnabled = await this.isEnabled();

      this.$set(this, 'enabled', isEnabled);
      await this.loadStatus();

      if (isEnabled) {
        const isUpgradeAvailable = await this.isUpgradeAvailable();

        this.$set(this, 'upgradeable', isUpgradeAvailable || window.location.search.includes('upgradeable'));
      }
    }
  },

  watch: {
    async editing() {
      if (this.editing && this.getConfig) {
        try {
          this.$set(this, 'loadingConfig', true);
          this.$set(this, 'config', await this.getConfig());
        } finally {
          this.$set(this, 'loadingConfig', false);
        }
      }
    }
  }
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <header>
      <h1>{{ title }}</h1>

      <div v-if="(enabled && !editing && $slots.details && !loadingConfig) ">
        <button class="btn role-secondary mr-5" @click="editFn">
          Edit Config
        </button>
        <button class="btn bg-error" @click="disableFn">
          Uninstall
        </button>
      </div>

      <button v-if="enabled && (editing || !$slots.details) && !loadingConfig" class="btn bg-error" @click="disableFn">
        Uninstall
      </button>
    </header>
    <Banner v-if="((enabled || editing) && status && status.message)" :color="status.state" class="mt-0">
      {{ status.message }}
    </Banner>
    <Banner v-if="(enabled && upgradeable)" color="success" class="mt-0">
      <div class="banner-message">
        <span>There's an upgrade available for your cluster.</span>
        <button class="btn bg-success" @click="upgrade">
          Upgrade Now
        </button>
      </div>
    </Banner>
    <div v-if="(!enabled && !editing)" class="body">
      <div class="not-enabled">
        <h4>{{ title }} is not currently installed. Installing it will use additional resources on this cluster.</h4>
        <button class="btn role-primary" @click="enable">
          Install
        </button>
      </div>
    </div>
    <div v-if="(enabled && !editing)" class="body">
      <slot name="details">
        <LoadingSpinner v-if="loadingConfig" />
        <slot v-else name="editing" />
        <div v-if="!loadingConfig" class="resource-footer mt-20">
          <button class="btn role-secondary mr-10" @click="cancel">
            Cancel
          </button>
          <AsyncButton mode="edit" @click="saveFn" />
        </div>
      </slot>
    </div>
    <div v-if="(editing || (enabled && !showDetail))" class="body">
      <LoadingSpinner v-if="loadingConfig" />
      <slot v-else name="editing" />
      <div v-if="editing && !loadingConfig" class="resource-footer mt-20">
        <button class="btn role-secondary mr-10" @click="cancel">
          Cancel
        </button>
        <AsyncButton mode="edit" @click="saveFn" />
      </div>
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

.banner-message {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

.border {
    border-bottom: 1px solid #dcdee7;
    padding-bottom: 15px;
}

header {
  width: 100%;
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

::v-deep {
  .nowrap {
    white-space: nowrap;
  }

  .monospace {
    font-family: $mono-font;
  }

  .cluster-status {
    padding-left: 40px;
  }

  .capability-status {
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
  }

  .not-enabled {
    text-align: center;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    height: 100%;
  }

  .enabled {
    width: 100%;
  }
}
</style>

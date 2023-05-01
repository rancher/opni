<script>
import { Card } from '@components/Card';
import UnitInput from '@shell/components/form/UnitInput';
import { get, set } from '../utils/object';

export default {
  components: { Card, UnitInput },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  methods: {
    updateUnit(key, suffix, value = 0) {
      set(this, `value.${ key }`, `${ value || 0 }${ suffix }`);
    },

    getUnit(key, suffix) {
      try {
        const value = get(this, `value.${ key }`);

        if (value.includes('Gi') && suffix === 'Mi') {
          return value.replace('Gi', '') * 1024;
        }

        return value.replace(suffix, '');
      } catch (ex) {
        return '';
      }
    }
  },

  computed: {
    memory: {
      get() {
        return this.getUnit('Resources.Limits.Memory', 'Mi');
      },

      set(value) {
        return this.updateUnit('Resources.Limits.Memory', 'Mi', value);
      }
    },
    cpuLimit: {
      get() {
        return this.getUnit('Resources.Limits.CPU', 'm');
      },

      set(value) {
        return this.updateUnit('Resources.Limits.CPU', 'm', value);
      }
    },
    cpuRequired: {
      get() {
        return this.getUnit('Resources.Requests.CPU', 'm');
      },

      set(value) {
        return this.updateUnit('Resources.Requests.CPU', 'm', value);
      }
    },
  }
};
</script>
<template>
  <div>
    <Card class="m-0 mt-20 mb-20" :show-highlight-border="false" :show-actions="false">
      <header v-if="value.Enabled" slot="title" class="text-default-text">
        <h3>Dashboard</h3>
        <button v-if="value.Enabled" class="btn role-secondary" @click="$emit('disable')">
          Disable Dashboard
        </button>
      </header>
      <div v-if="value.Enabled" slot="body">
        <div class="row border">
          <div class="col span-12">
            <div class="col span-4">
              <UnitInput v-model="value.Replicas" label="Replicas" :suffix="false" />
            </div>
          </div>
        </div>
        <div class="row mt-10">
          <div class="col span-12">
            <h4>Resources</h4>
          </div>
        </div>
        <div class="row">
          <div class="col span-4">
            <UnitInput v-model="memory" label="Memory" suffix="MiB" />
          </div>
          <div class="col span-4">
            <UnitInput v-model="cpuLimit" label="CPU Limit" suffix="miliCpu" />
          </div>
          <div class="col span-4">
            <UnitInput v-model="cpuRequired" label="CPU Required" suffix="miliCpu" />
          </div>
        </div>
      </div>
      <div v-else slot="body" class="not-enabled">
        <h5>The Dashboard is not currently enabled. Enabling it will install additional resources.</h5>
        <button class="btn role-primary" @click="$emit('enable')">
          Enable
        </button>
      </div>
    </Card>
  </div>
</template>

<style lang="scss" scoped>
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

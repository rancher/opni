<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';

export default {
  components: {
    LabeledInput,
    LabeledSelect,
    UnitInput
  },

  props: {
    value: {
      type:    Object,
      default: undefined
    },

    title: {
      type:     String,
      required: true
    },

    disabled: {
      type:    Boolean,
      default: false
    }
  },

  async fetch() {
    try {
      await this.load();
    } catch (ex) {}
  },

  data() {
    return {
      source:  (this.value || {}).imageSource ? 'imageSource' : 'httpSource',
      options: [
        {
          label: 'HTTP',
          value: 'httpSource'
        },
        {
          label: 'Image',
          value: 'imageSource'
        },
      ]
    };
  },

  methods: {},

  computed: {},

  watch: {
    source(val) {
      const target = val === 'httpSource' ? 'httpSource' : 'imageSource';

      this.$set(this.value, target, undefined);
    }
  }
};
</script>
<template>
  <div>
    <h4>{{ title }}</h4>
    <div class="row mb-20">
      <div class="col span-4">
        <LabeledSelect v-model="source" :options="options" label="Model Source Target" :disabled="disabled" />
      </div>
      <div v-if="source === 'httpSource'" class="col span-4">
        <LabeledInput v-model="(value || {}).httpSource" label="Source" :disabled="disabled" />
      </div>
      <div v-else class="col span-4">
        <LabeledInput v-model="(value || {}).imageSource" label="Source" :disabled="disabled" />
      </div>
      <div class="col span-4">
        <UnitInput v-model="(value || {}).replicas" label="Replicas" :suffix="false" :disabled="disabled" />
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.middle {
  display: flex;
  flex-direction: row;
  align-items: center;
}

.banner {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}

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
  .card-container {
    min-height: initial;
  }

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

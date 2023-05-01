<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import UnitInput from '@shell/components/form/UnitInput';
import ArrayListSelect from '@shell/components/form/ArrayListSelect';
import { Checkbox } from '@components/Form/Checkbox';
import TextAreaAutoGrow from '@components/Form/TextArea/TextAreaAutoGrow';
import { Severity } from '../models/alerting/Condition';
import { getAlertEndpoints } from '../utils/requests/alerts';

export function createDefaultAttachedEndpoints() {
  return {
    items:              [],
    initialDelay:       '10s',
    repeatInterval:     '600s',
    throttlingDuration: '600s',
    details:            {
      title: '', body: '', sendResolved: false
    }
  };
}

export default {
  components: {
    ArrayListSelect,
    Checkbox,
    LabeledInput,
    LabeledSelect,
    TextAreaAutoGrow,
    UnitInput,
  },

  props: {
    value: {
      type:     Object,
      required: true
    },

    showSeverity: {
      type:     Boolean,
      required: true
    },

    severity: {
      type:    Number,
      default: 0
    }
  },

  async fetch() {
    await this.load();
  },

  data() {
    return {
      options: {
        severityOptions: [
          {
            label: 'Info',
            value: Severity.INFO
          },
          {
            label: 'Warning',
            value: Severity.WARNING
          },
          {
            label: 'Error',
            value: Severity.ERROR
          },
          {
            label: 'Critical',
            value: Severity.CRITICAL
          },
        ],
        endpointOptions: [],
      },
    };
  },

  methods: {
    async load() {
      const endpoints = await getAlertEndpoints(this);

      this.$set(this.options, 'endpointOptions', endpoints.map(e => ({
        label: e.nameDisplay,
        value: e.id
      })));
    },
  },

  computed: {
    attachedEndpoints: {
      get() {
        return this.value.items.map(item => item.endpointId);
      },

      set(value) {
        this.$emit('input', { ...this.value, items: value.map(v => ({ endpointId: v })) });
      }
    },

    initialDelay: {
      get() {
        return Number.parseInt(this.value.initialDelay || '0');
      },

      set(value) {
        this.$emit('input', { ...this.value, initialDelay: `${ (value || 0) }s` });
      }
    },

    repeatInterval: {
      get() {
        return Number.parseInt(this.value.repeatInterval || '0') / 60;
      },

      set(value) {
        this.$emit('input', { ...this.value, repeatInterval: `${ Math.round(value * 60 || 0) }s` });
      }
    },

    throttlingDuration: {
      get() {
        return Number.parseInt(this.value.throttlingDuration || '0') / 60;
      },

      set(value) {
        this.$emit('input', { ...this.value, throttlingDuration: `${ Math.round(value * 60 || 0) }s` });
      }
    },
    showMessageOptions() {
      return this.value.items.length > 0 && this.value.items.some(item => item?.endpointId);
    }
  }
};
</script>
<template>
  <div class="attached-endpoints">
    <div class="row mt-10">
      <div class="col span-12">
        <ArrayListSelect v-model="attachedEndpoints" add-label="Add Endpoint" :options="options.endpointOptions" />
      </div>
    </div>
    <div v-if="showMessageOptions">
      <div class="row mt-10">
        <div class="col span-4">
          <UnitInput v-model="initialDelay" label="Initial Delay" suffix="s" />
        </div>
        <div class="col span-4">
          <UnitInput v-model="repeatInterval" label="Repeat Interval" suffix="m" />
        </div>
        <div class="col span-4">
          <UnitInput v-model="throttlingDuration" label="Throttling Duration" suffix="m" />
        </div>
      </div>
      <h4 class="mt-20">
        Message
      </h4>
      <div class="row mt-10">
        <div class="col" :class="{'span-6': showSeverity, 'span-12': !showSeverity}">
          <LabeledInput v-model="value.details.title" label="Title" :required="true" />
        </div>
        <div v-if="showSeverity" class="col span-6">
          <LabeledSelect :value="severity" label="Severity" :options="options.severityOptions" @input="(val) => $emit('severity', val)" />
        </div>
      </div>
      <div class="row mt-10">
        <div class="col span-12">
          <TextAreaAutoGrow v-model="value.details.body" :min-height="250" :required="true" />
        </div>
      </div>
      <div class="row mt-10">
        <div class="col span-12">
          <Checkbox v-model="value.details.sendResolved" label="Send Resolved" />
        </div>
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

.bottom {
  border-bottom: 1px solid var(--header-border);
  padding-bottom: 20px;
}
</style>

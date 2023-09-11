<script>
import { Checkbox } from '@components/Form/Checkbox';
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';

const DEFAULT_MIN = 'TLS12';
const DEFAULT_MAX = 'TLS13';

export default {
  components: {
    Checkbox, LabeledInput, LabeledSelect
  },

  props: {
    value: {
      type:     Object,
      default: () => null
    }
  },

  data() {
    if (this.value && !this.value.minVersion) {
      this.$set(this.value, 'minVersion', DEFAULT_MIN);
    }

    if (this.value && !this.value.maxVersion) {
      this.$set(this.value, 'maxVersion', DEFAULT_MAX);
    }

    return {
      tlsOptions: [
        {
          label: 'TLS 1.0',
          value: 'TLS10'
        },
        {
          label: 'TLS 1.1',
          value: 'TLS11'
        },
        {
          label: 'TLS 1.2',
          value: 'TLS12'
        },
        {
          label: 'TLS 1.3',
          value: 'TLS13'
        },
      ]
    };
  },
  computed: {
    enabled: {
      get() {
        return !!this.value;
      },
      set(value) {
        this.$emit('input', value ? { minVersion: DEFAULT_MIN, maxVersion: DEFAULT_MAX } : undefined);
      }
    }
  }
};
</script>
<template>
  <div>
    <h4 class="middle">
      TLS <Checkbox v-model="enabled" class="ml-15" />
    </h4>
    <div v-if="enabled">
      <div class="row">
        <div class="col span-6">
          <LabeledSelect v-model="value.minVersion" :options="tlsOptions" label="TLS Min Version" />
        </div>
        <div class="col span-6">
          <LabeledSelect v-model="value.maxVersion" :options="tlsOptions" label="TLS Max Version" />
        </div>
      </div>
      <div class="row mt-10">
        <div class="col span-6">
          <LabeledInput v-model="value.serverName" label="Server Name" />
        </div>
        <div class="col span-6 middle">
          <Checkbox v-model="value.insecureSkipVerify" label="Skip Server Cert Validation" />
        </div>
      </div>
      <div class="row mt-10">
        <div class="col span-4">
          <LabeledInput v-model="value.caFile" label="CA File" />
        </div>
        <div class="col span-4">
          <LabeledInput v-model="value.certFile" label="Cert File" />
        </div>
        <div class="col span-4">
          <LabeledInput v-model="value.keyFile" label="Key File" />
        </div>
      </div>
    </div>
  </div>
</template>

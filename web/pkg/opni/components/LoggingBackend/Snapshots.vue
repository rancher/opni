<script>
import UnitInput from '@shell/components/form/UnitInput';
import { Checkbox } from '@components/Form/Checkbox';
import { LabeledInput } from '@components/Form/LabeledInput';
import EnabledDisabled from '../EnabledDisabled';

export default {
  components: {
    Checkbox, EnabledDisabled, LabeledInput, UnitInput
  },

  props: {
    value: {
      type:     Object,
      default: () => null
    }
  },

  created() {
    if (!this.value) {
      return;
    }

    if (!this.value.credentials) {
      this.$set(this.value, 'credentials', { });
    }
  },

  computed: {
    enabled: {
      get() {
        return !!this.value;
      },

      set(value) {
        this.$emit('input', value ? { credentials: {} } : null);
      }
    },

    proxyEnabled: {
      get() {
        return !!this.value.proxySettings;
      },

      set(value) {
        this.$set(this.value, 'proxySettings', value ? {} : undefined);
      }
    }
  }
};
</script>
<template>
  <EnabledDisabled
    v-model="enabled"
    disabled-message="S3 Snapshots are not currently enabled."
  >
    <div v-if="value">
      <h4>
        Access
      </h4>
      <div class="row border-bottom mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.endpoint" label="Endpoint" />
        </div>
        <div class="col span-3 middle">
          <Checkbox v-model="value.insecure" label="Insecure" />
        </div>
        <div class="col span-3 middle">
          <Checkbox v-model="value.pathStyleAccess" label="Path Style Access" />
        </div>
      </div>
      <div class="row border-bottom mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.credentials.accessKey" label="Access Key" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.credentials.secretKey" label="Secret Key" />
        </div>
      </div>
      <h4>
        Target
      </h4>
      <div class="row border-bottom mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.bucket" label="Bucket" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.folder" label="Folder" />
        </div>
      </div>
      <h4 class="middle">
        Proxy <Checkbox v-model="proxyEnabled" class="ml-15" />
      </h4>
      <div v-if="proxyEnabled" class="row border-bottom mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.proxySettings.proxyHost" label="Host" />
        </div>
        <div class="col span-6">
          <UnitInput v-model="value.proxySettings.proxyPort" base-unit="" label="Port" />
        </div>
      </div>
    </div>
  </EnabledDisabled>
</template>

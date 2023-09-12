<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import KeyValue from '@shell/components/form/KeyValue';
import ArrayList from '@shell/components/form/ArrayList';
import Tls from '../Tls';

export default {
  components: {
    LabeledInput, LabeledSelect, Tls, KeyValue, ArrayList
  },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  data() {
    const clientSecretTypes = [
      {
        label: 'Client Secret',
        value: 'clientSecret'
      },
      {
        label: 'Client Secret File',
        value: 'clientSecretFile'
      }
    ];

    const clientSecretType = clientSecretTypes.map(pt => pt.value).find(type => this.value[type]) || clientSecretTypes[0].value;

    return {
      clientSecretTypes,
      clientSecretType
    };
  },
  watch: {
    clientSecretType() {
      this.$set(this.value, 'clientSecret', '');
      this.$set(this.value, 'clientSecretFile', '');
    }
  }
};
</script>
<template>
  <div>
    <h5>Client</h5>
    <div class="row">
      <div class="col span-12">
        <LabeledInput v-model="value.clientId" label="Client Id" />
      </div>
    </div>
    <div class="row mt-10 mb-10">
      <div class="col span-6">
        <LabeledSelect v-model="clientSecretType" :options="clientSecretTypes" label="Client Secret Type" />
      </div>
      <div class="col span-6">
        <LabeledInput v-if="clientSecretType === 'clientSecret'" v-model="value.clientSecret" label="Client Secret" type="password" />
        <LabeledInput v-else v-model="value.clientSecretFile" label="Client Secret File" />
      </div>
    </div>
    <div class="row mb-10">
      <div class="col span-6">
      </div>
    </div>

    <h5>Token</h5>
    <div class="row">
      <div class="col span-12">
        <LabeledInput v-model="value.tokenUrl" label="Token URL" />
      </div>
    </div>
    <div class="row mt-10">
      <div class="col span-6">
        <div>Token URL Parameters</div>
        <KeyValue
          v-model="value.endpointParams"
          :read-allowed="false"
          :value-multiline="false"
          add-label="Add Token URL Parameter"
        />
      </div>
      <div class="col span-6">
        <div class="mb-10">
          Token Scopes
        </div>
        <ArrayList v-model="value.scopes" class="scopes" label="Token Scopes" add-label="Add Token Scope" />
      </div>
    </div>
    <div class="row mt-10 mb-10">
    </div>

    <h5>Target</h5>
    <div class="row mt-10 mb-10">
      <div class="col span-12">
        <LabeledInput v-model="value.proxyUrl" label="Proxy Url" />
      </div>
    </div>
    <Tls v-model="value.tlsConfig" />
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .key-value {
    .kv-item {
      margin-bottom: 0;
      input {
        height: 61px;
      }
    }

    .text-label {
      height: 0px;
      opacity: 0;
    }
  }

  .scopes {
    .footer {
      margin-top: 20px;
    }
  }
}
</style>

<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { Checkbox } from '@components/Form/Checkbox';
import BasicAuth from './Auths/BasicAuth';
import Authorization from './Auths/Authorization';
import OAuth2 from './Auths/OAuth2';
import Tls from './Tls';

export default {
  components: {
    Authorization, BasicAuth, Checkbox, LabeledInput, LabeledSelect, OAuth2, Tls
  },

  props: {
    value: {
      type:     Object,
      required: true
    }
  },

  data() {
    const authorizationTypes = [
      {
        label:   'Basic Authorization',
        value:   'basicAuth',
      },
      {
        label: 'Authorization Header',
        value: 'authorization',
      },
      {
        label:   'OAuth2',
        value:   'oauth2',
      },
    ];

    if (!this.value.httpConfig) {
      this.$set(this.value, 'httpConfig', {
        ...{ tlsConfig: {} },
        ...(this.value.httpConfig || {})
      });
    }

    const authorizationType = this.getAuthType(this.value.httpConfig);

    if (this.$route.name === 'endpoint-create') {
      this.setAuthDefault(authorizationType, authorizationTypes);
    }

    return {
      authorizationTypes,
      authorizationType,
    };
  },
  watch: {
    authorizationType(newType, oldType) {
      this.setAuthDefault(newType, this.authorizationTypes);
      this.$set(this.value.httpConfig, oldType, null);
    }
  },
  methods: {
    setAuthDefault(type, authTypes) {
      this.$set(this.value.httpConfig, type, authTypes.find(authType => authType.value === type)?.default || {});
    },
    getAuthType(httpConfig) {
      if (httpConfig.authorization) {
        return 'authorization';
      }

      if (httpConfig.oauth2) {
        return 'oauth2';
      }

      return 'basicAuth';
    },
  },

  computed: {
    authEnabled: {
      get() {
        return this.authorizationTypes.some(type => this.value.httpConfig[type.value]);
      },

      set(value) {
        if (value) {
          this.setAuthDefault(this.authorizationTypes[0].value, this.authorizationTypes);
        } else {
          this.authorizationTypes.forEach(type => this.$set(this.value.httpConfig, type.value, null));
        }
      }
    }
  }
};
</script>
<template>
  <div>
    <div class="row mt-20 bottom mb-10">
      <div class="col span-12">
        <LabeledInput v-model="value.maxAlerts" label="Max Alerts" tooltip="The maximum number of alerts to include in a single webhook message. When the value is 0, all alerts are included." />
      </div>
    </div>
    <h4>Networking</h4>
    <div class="row mt-10 mb-10">
      <div class="col span-6">
        <LabeledInput v-model="value.url" label="URL" :required="true" />
      </div>
      <div class="col span-2 middle">
        <Checkbox v-model="value.httpConfig.enabledHttp2" label="Use HTTP2" />
      </div>
      <div class="col span-3 middle">
        <Checkbox v-model="value.httpConfig.followRedirects" label="Follow Redirects" />
      </div>
    </div>
    <Tls v-model="value.httpConfig.tlsConfig" class="mb-10" />
    <h4>Proxy</h4>
    <div class="row mt-10 mb-10">
      <div class="col span-12">
        <LabeledInput v-model="value.httpConfig.proxyUrl" label="Proxy URL" />
      </div>
    </div>
    <h4 class="middle">
      Authorization <Checkbox v-model="authEnabled" class="ml-15" />
    </h4>
    <div v-if="authEnabled">
      <div class="row mt-10">
        <LabeledSelect v-model="authorizationType" :options="authorizationTypes" label="Authorization" />
      </div>
      <BasicAuth v-if="authorizationType === 'basicAuth'" v-model="value.httpConfig.basicAuth" class="mt-10" />
      <Authorization v-if="authorizationType === 'authorization'" v-model="value.httpConfig.authorization" class="mt-10" />
      <OAuth2 v-if="authorizationType === 'oauth2'" v-model="value.httpConfig.oauth2" class="mt-10" />
    </div>
  </div>
</template>

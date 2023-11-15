<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import { Checkbox } from '@components/Form/Checkbox';
import UnitInput from '@shell/components/form/UnitInput';
import { createComputedDuration } from '@pkg/opni/utils/computed';

export default {
  components: {
    Checkbox, UnitInput, LabeledInput
  },

  props: {
    value: {
      type:     Object,
      required: true
    },
  },

  created() {
  },

  data() {
    return {};
  },

  computed: {
    idleConnTimeout:       createComputedDuration('value.cortexConfig.storage.azure.http.idleConnTimeout'),
    responseHeaderTimeout: createComputedDuration('value.cortexConfig.storage.azure.http.responseHeaderTimeout'),
    tlsHandshakeTimeout:   createComputedDuration('value.cortexConfig.storage.azure.http.tlsHandshakeTimeout'),
    expectContinueTimeout: createComputedDuration('value.cortexConfig.storage.azure.http.expectContinueTimeout'),
  },

  methods: {},
  watch:   {},
};
</script>
<template>
  <div class="m-0">
    <div>
      <h3>Target</h3>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.endpointSuffix" label="Endpoint Suffix" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.containerName" label="Container Name" />
        </div>
      </div>
      <h3>Access</h3>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.accountName" label="Account Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.accountKey" label="Account Key" type="password" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.userAssignedId" label="User Assigned Id" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.azure.msiResource" label="MSI Resource" />
        </div>
      </div>

      <h3>Connection</h3>
      <div class="row mb-10">
        <div class="col span-4">
          <UnitInput v-model="idleConnTimeout" label="Idle Connection Timeout" placeholder="e.g. 30, 60" suffix="s" />
        </div>
        <div class="col span-4">
          <UnitInput
            v-model="responseHeaderTimeout"
            label="Response Header Timeout"
            placeholder="e.g. 30, 60"
            suffix="s"
          />
        </div>
        <div class="col span-4">
          <UnitInput
            v-model="value.cortexConfig.storage.azure.maxRetries"
            label="Max Retries"
            placeholder="5"
            suffix=""
            :input-exponent="0"
            base-unit=""
          />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-4">
          <UnitInput
            v-model="tlsHandshakeTimeout"
            label="TLS Handshake Timeout"
            placeholder="e.g. 30, 60"
            suffix="s"
          />
        </div>
        <div class="col span-4 middle">
          <Checkbox v-model="value.cortexConfig.storage.azure.http.insecureSkipVerify" label="Insecure Skip Verify" />
        </div>
        <div class="col span-4">
          <UnitInput
            v-model="expectContinueTimeout"
            label="Expect Continue Timeout"
            placeholder="e.g. 30, 60"
            suffix="s"
          />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-4">
          <UnitInput v-model="value.cortexConfig.storage.azure.http.maxIdleConnections" label="Max Idle Connections" suffix="" :input-exponent="0" base-unit="" />
        </div>
        <div class="col span-4">
          <UnitInput
            v-model="value.cortexConfig.storage.azure.http.maxIdleConnectionsPerHost"
            label="Max Idle Connections Per Host"
            suffix=""
            :input-exponent="0"
            base-unit=""
          />
        </div>
        <div class="col span-4">
          <UnitInput v-model="value.cortexConfig.storage.azure.http.maxConnectionsPerHost" label="Max Connections Per Host" suffix="" :input-exponent="0" base-unit="" />
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
header {
  width: 100%;
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0;
}

::v-deep {
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

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
    connectTimeout: createComputedDuration('value.cortexConfig.storage.swift.connectTimeout'),
    requestTimeout: createComputedDuration('value.cortexConfig.storage.swift.requestTimeout'),
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
          <LabeledInput v-model="value.cortexConfig.storage.swift.regionName" label="Region Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.containerName" label="Container Name" />
        </div>
      </div>
      <div class="row mb-10 border">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.projectName" label="Project Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.projectId" label="Project Id" />
        </div>
      </div>
      <div class="row mb-10 border">
        <div class="col span-6 middle">
          <LabeledInput v-model="value.cortexConfig.storage.swift.projectDomainName" label="Project Domain Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.projectDomainId" label="Project Domain Id" />
        </div>
      </div>
      <h3>Access</h3>
      <div class="row mb-10">
        <div class="col span-6">
          <UnitInput
            v-model="value.cortexConfig.storage.swift.authVersion"
            label="Auth Version"
            placeholder="e.g. 3"
            suffix=""
            :input-exponent="0"
            base-unit=""
          />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.authUrl" label="Auth Url" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.username" label="Username" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.userId" label="User Id" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.password" label="Password" type="password" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.applicationCredentialId" label="Application Credential Id" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.applicationCredentialName" label="Application Credential Name" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.applicationCredentialSecret" label="Application Credential Secret" type="password" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.userDomainName" label="User Domain Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.userDomainId" label="User Domain Id" />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.domainName" label="Domain Name" />
        </div>
        <div class="col span-6">
          <LabeledInput v-model="value.cortexConfig.storage.swift.domainId" label="Domain Id" />
        </div>
      </div>

      <h3>Connection</h3>
      <div class="row mb-10">
        <div class="col span-6">
          <UnitInput v-model="connectTimeout" label="Connection Timeout" placeholder="e.g. 30, 60" suffix="s" />
        </div>
        <div class="col span-6">
          <UnitInput
            v-model="requestTimeout"
            label="Request Timeout"
            placeholder="e.g. 30, 60"
            suffix="s"
          />
        </div>
      </div>
      <div class="row mb-10">
        <div class="col span-6">
          <UnitInput
            v-model="value.cortexConfig.storage.swift.maxRetries"
            label="Max Retries"
            placeholder="e.g. 5"
            suffix=""
            :input-exponent="0"
            base-unit=""
          />
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

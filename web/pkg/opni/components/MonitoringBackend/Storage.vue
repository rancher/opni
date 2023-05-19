<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { Checkbox } from '@components/Form/Checkbox';
import UnitInput from '@shell/components/form/UnitInput';
import { DeploymentMode, StorageBackend } from '../../utils/requests/monitoring';

export const SECONDS_IN_DAY = 86400;

export default {
  components: {
    Checkbox, UnitInput, LabeledInput, LabeledSelect
  },

  props: {
    value: {
      type:     Object,
      required: true
    },
  },

  created() {
    if (!this.value.storage.s3?.endpoint) {
      this.updateEndpoint();
    }
  },

  data() {
    return {
      signatureVersionOptions: [
        {
          label: 'v4',
          value: 'v4'
        },
        {
          label: 'v2',
          value: 'v2'
        },
      ],
      regions: [
        'us-east-2',
        'us-east-1',
        'us-west-1',
        'us-west-2',
        'af-south-1',
        'ap-east-1',
        'ap-southeast-3',
        'ap-south-1',
        'ap-northeast-3',
        'ap-northeast-2',
        'ap-southeast-1',
        'ap-southeast-2',
        'ap-northeast-1',
        'ca-central-1',
        'eu-central-1',
        'eu-west-1',
        'eu-west-2',
        'eu-south-1',
        'eu-west-3',
        'eu-north-1',
        'me-south-1',
        'me-central-1',
        'sa-east-1',
        'us-gov-east-1',
        'us-gov-west-1',
      ],
      sseTypes: [
        { label: 'None', value: '' },
        { label: 'SSE-KMS', value: 'SSE-KMS' },
        { label: 'SSE-S3', value: 'SSE-S3' },
      ],
      SECONDS_IN_DAY,
      StorageBackend,
      DeploymentMode,
    };
  },

  computed: {
    storageOptions() {
      // only enable filesystem in standalone mode (0)
      if (this.value.mode === DeploymentMode.AllInOne) {
        return [
          { label: 'Filesystem', value: StorageBackend.Filesystem },
          { label: 'S3', value: StorageBackend.S3 }
        ];
      }

      return [
        { label: 'S3', value: StorageBackend.S3 }
      ];
    },

    s3RetentionPeriod: {
      get() {
        return Number.parseInt(this.value.storage.retentionPeriod || '0') / SECONDS_IN_DAY;
      },

      set(value) {
        this.$set(this.value.storage, 'retentionPeriod', `${ (value || 0) * SECONDS_IN_DAY }s`);
      }
    },

    s3IdleConnTimeout: {
      get() {
        return Number.parseInt(this.value.storage.s3?.http?.idleConnTimeout || '0');
      },

      set(value) {
        this.$set(this.value.storage.s3.http, 'idleConnTimeout', `${ value || 0 }s`);
      }
    },

    s3ResponseHeaderTimeout: {
      get() {
        return Number.parseInt(this.value.storage.s3?.http?.responseHeaderTimeout || '0');
      },

      set(value) {
        this.$set(this.value.storage.s3.http, 'responseHeaderTimeout', `${ value || 0 }s`);
      }
    },

    s3TlsHandshakeTimeout: {
      get() {
        return Number.parseInt(this.value.storage.s3?.http?.tlsHandshakeTimeout || '0');
      },

      set(value) {
        this.$set(this.value.storage.s3.http, 'tlsHandshakeTimeout', `${ value || 0 }s`);
      }
    },

    s3ExpectContinueTimeout: {
      get() {
        return Number.parseInt(this.value.storage.s3?.http?.expectContinueTimeout || '0');
      },

      set(value) {
        this.$set(this.value.storage.s3.http, 'expectContinueTimeout', `${ value || 0 }s`);
      }
    },
  },

  methods: {
    updateEndpoint() {
      const endpoints = {
        'us-east-2':      's3.us-east-2.amazonaws.com',
        'us-east-1':      's3.us-east-1.amazonaws.com',
        'us-west-1':      's3.us-west-1.amazonaws.com',
        'us-west-2':      's3.us-west-2.amazonaws.com',
        'af-south-1':     's3.af-south-1.amazonaws.com',
        'ap-east-1':      's3.ap-east-1.amazonaws.com',
        'ap-south-1':     's3.ap-south-1.amazonaws.com',
        'ap-northeast-3': 's3.ap-northeast-3.amazonaws.com',
        'ap-northeast-2': 's3.ap-northeast-2.amazonaws.com',
        'ap-southeast-1': 's3.ap-southeast-1.amazonaws.com',
        'ap-southeast-2': 's3.ap-southeast-2.amazonaws.com',
        'ap-northeast-1': 's3.ap-northeast-1.amazonaws.com',
        'ca-central-1':   's3.ca-central-1.amazonaws.com',
        'cn-northwest-1': 's3.cn-northwest-1.amazonaws.com.cn',
        'eu-central-1':   's3.eu-central-1.amazonaws.com',
        'eu-west-1':      's3.eu-west-1.amazonaws.com',
        'eu-west-2':      's3.eu-west-2.amazonaws.com',
        'eu-south-1':     's3.eu-south-1.amazonaws.com',
        'eu-west-3':      's3.eu-west-3.amazonaws.com',
        'eu-north-1':     's3.eu-north-1.amazonaws.com',
        'ap-southeast-3': 's3.ap-southeast-3.amazonaws.com',
        'me-south-1':     's3.me-south-1.amazonaws.com',
        'me-central-1':   's3.me-central-1.amazonaws.com',
        'sa-east-1':      's3.sa-east-1.amazonaws.com',
        'us-gov-east-1':  's3.us-gov-east-1.amazonaws.com',
        'us-gov-west-1':  's3.us-gov-west-1.amazonaws.com',
      };

      if (this.value.storage.s3?.region) {
        return this.$set(this.value.storage.s3, 'endpoint', `${ endpoints[this.value.storage.s3.region] }`);
      }
    },
    watch: {
      storageOptions() {
        const vals = this.storageOptions.map(so => so.value);

        if (!vals.includes(this.value.storage.backend)) {
          this.$set(this.value.storage, 'backend', vals[0]);
        }
      }
    },
  }
};
</script>
<template>
  <div class="m-0">
    <div>
      <div class="row" :class="{ border: value.storage.backend === StorageBackend.S3 }">
        <div class="col span-6">
          <LabeledSelect v-model="value.storage.backend" :options="storageOptions" label="Storage Type" />
        </div>
        <div class="col span-6">
          <UnitInput
            v-model="s3RetentionPeriod"
            class="retention-period"
            label="Data Retention Period"
            suffix="days"
            tooltip="A value of 0 will retain data indefinitely"
          />
        </div>
      </div>
      <div v-if="value.storage.backend === StorageBackend.S3" class="mt-15">
        <h3>Target</h3>
        <div class="row mb-10">
          <div class="col span-6">
            <LabeledSelect v-model="value.storage.s3.region" :options="regions" label="Region" @input="updateEndpoint" />
          </div>
          <div class="col span-6">
            <LabeledInput v-model="value.storage.s3.bucketName" label="Bucket Name" :required="true" />
          </div>
        </div>
        <div class="row mb-10 border">
          <div class="col span-6">
            <LabeledInput v-model="value.storage.s3.endpoint" label="Endpoint" :required="true" />
          </div>
          <div class="col span-6 middle">
            <Checkbox v-model="value.storage.s3.insecure" label="Insecure" />
          </div>
        </div>
        <h3>Access</h3>
        <div class="row mb-10">
          <div class="col span-6">
            <LabeledInput v-model="value.storage.s3.accessKeyID" label="Access Key ID" :required="true" />
          </div>
          <div class="col span-6">
            <LabeledInput
              v-model="value.storage.s3.secretAccessKey"
              label="Secret Access Key"
              :required="true"
              type="password"
            />
          </div>
        </div>
        <div class="row mb-10">
          <div class="col span-6">
            <LabeledSelect
              v-model="value.storage.s3.signatureVersion"
              :options="signatureVersionOptions"
              label="Signature Version"
            />
          </div>
        </div>
        <h3>Server Side Encryption</h3>
        <div class="row mb-10">
          <div class="col span-6">
            <LabeledSelect v-model="value.storage.s3.sse.type" :options="sseTypes" label="Type" />
          </div>
        </div>
        <div v-if="value.storage.s3.sse.type === 'SSE-KMS'" class="row mb-10">
          <div class="col span-6">
            <LabeledInput v-model="value.storage.s3.sse.kmsKeyID" label="KMS Key Id" :required="true" />
          </div>
          <div class="col span-6">
            <LabeledInput
              v-model="value.storage.s3.sse.kmsEncryptionContext"
              label="KMS Encryption Context"
              :required="true"
            />
          </div>
        </div>
        <h3>Connection</h3>
        <div class="row mb-10">
          <div class="col span-6">
            <UnitInput v-model="s3IdleConnTimeout" label="Idle Connection Timeout" placeholder="e.g. 30, 60" suffix="s" />
          </div>
          <div class="col span-6">
            <UnitInput
              v-model="s3ResponseHeaderTimeout"
              label="Response Header Timeout"
              placeholder="e.g. 30, 60"
              suffix="s"
            />
          </div>
        </div>
        <div class="row mb-10">
          <div class="col span-4">
            <UnitInput
              v-model="s3TlsHandshakeTimeout"
              label="TLS Handshake Timeout"
              placeholder="e.g. 30, 60"
              suffix="s"
            />
          </div>
          <div class="col span-3 middle">
            <Checkbox v-model="value.storage.s3.http.insecureSkipVerify" label="Insecure Skip Verify" />
          </div>
          <div class="col span-5">
            <UnitInput
              v-model="s3ExpectContinueTimeout"
              label="Expect Continue Timeout"
              placeholder="e.g. 30, 60"
              suffix="s"
            />
          </div>
        </div>
        <div class="row mb-10">
          <div class="col span-4">
            <UnitInput v-model="value.storage.s3.http.maxIdleConns" label="Max Idle Connections" suffix="" />
          </div>
          <div class="col span-4">
            <UnitInput
              v-model="value.storage.s3.http.maxIdleConnsPerHost"
              label="Max Idle Connections Per Host"
              suffix=""
            />
          </div>
          <div class="col span-4">
            <UnitInput v-model="value.storage.s3.http.maxConnsPerHost" label="Max Connections Per Host" suffix="" />
          </div>
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

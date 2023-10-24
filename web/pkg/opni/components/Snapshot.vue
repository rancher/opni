<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import { Checkbox } from '@components/Form/Checkbox';
import Loading from '@shell/components/Loading';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import { LoggingAdmin } from '@pkg/opni/api/opni';
import ResourceFooter from '@pkg/opni/components/Layout/ResourceFooter';
import UnitInput from '@shell/components/form/UnitInput';
import ArrayList from '@shell/components/form/ArrayList';
import { createComputedDuration } from '@pkg/opni/utils/computed';
import { exceptionToErrorsArray } from '../utils/error';

export default {
  components: {
    ArrayList,
    Checkbox,
    LabeledInput,
    Loading,
    Tab,
    Tabbed,
    UnitInput,
    ResourceFooter
  },

  async fetch() {
    if (this.$route.params.id) {
      const ref = new LoggingAdmin.Types.SnapshotReference({ name: this.$route.params.id });
      const config = await LoggingAdmin.Service.GetSnapshotSchedule(ref);

      console.log('ggg', config);

      this.$set(this.config.ref, 'name', config.ref.name);
      this.$set(this.config, 'cronSchedule', config.cronSchedule);
      this.$set(this.config, 'additionalIndices', config.additionalIndices);
      if (config.retention) {
        this.$set(this.config, 'retention', {});
        this.$set(this.config.retention, 'timeRetention', config.retention.timeRetention);
        this.$set(this.config.retention, 'maxSnapshots', config.retention.maxSnapshots);
      }
    }
  },

  data() {
    return {
      error:            '',
      config: {
        ref:               { name: '' },
        cronSchedule:      '',
        additionalIndices: []
      }
    };
  },

  methods: {
    async save(buttonCallback) {
      if (this.config === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }

      try {
        const ref = new LoggingAdmin.Types.SnapshotReference(this.config.ref);
        const retention = this.config.retention ? new LoggingAdmin.Types.SnapshotRetention(this.config.retention) : undefined;
        const schedule = new LoggingAdmin.Types.SnapshotSchedule({
          ref,
          cronSchedule:      this.config.cronSchedule,
          retention,
          additionalIndices: this.config.additionalIndices
        });

        console.log('sss', schedule);
        await LoggingAdmin.Service.CreateOrUpdateSnapshotSchedule(schedule);
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);

        return;
      }

      this.$set(this, 'error', '');
      buttonCallback(true);
      this.$router.replace({ name: 'snapshots' });
    },

    cancel() {
      this.$router.replace({ name: 'snapshots' });
    },
  },

  computed: {
    retentionLimits: {
      get() {
        return !!this.config.retention;
      },

      set(value) {
        this.$set(this.config, 'retention', value ? { timeRetention: '7d', maxSnapshots: 5 } : undefined);
      }
    },

    timeRetention: {
      get() {
        return Number.parseInt(this.config.retention.timeRetention);
      },

      set(value) {
        this.$set(this.config.retention, 'timeRetention', `${ value }d`);
      }
    }
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else>
    <div class="row mb-20">
      <div class="col span-12">
        <LabeledInput
          v-model="config.ref.name"
          label="Name"
          :required="true"
        />
      </div>
    </div>
    <Tabbed :side-tabs="true" class="mb-20">
      <Tab
        name="options"
        label="Options"
        :weight="3"
      >
        <div class="row">
          <div class="col span-12">
            <LabeledInput v-model="config.cronSchedule" label="Cron Schedule" placeholder="0 */5 * * *" />
          </div>
        </div>
        <div class="row bottom">
          <div class="col span-12">
            <h4 class="middle mt-20">
              Additional Indices for Snapshot
            </h4>
            <ArrayList
              v-model="config.additionalIndices"
              :protip="false"
              add-label="Add Index"
            />
          </div>
        </div>
        <h4 class="middle mt-20">
          Retention Limits <Checkbox v-model="retentionLimits" class="ml-15" />
        </h4>
        <div v-if="retentionLimits" class="row mt-10">
          <div class="col span-6">
            <UnitInput v-model="timeRetention" label="Retention Time" suffix="Days" />
          </div>
          <div class="col span-6">
            <UnitInput v-model="config.retention.maxSnapshots" label="Max Snapshots" base-unit="" />
          </div>
        </div>
      </Tab>
    </Tabbed>
    <ResourceFooter :save="save" :cancel="cancel" :error="error" />
  </div>
</template>

<style lang="scss" scoped>
.install-command {
  width: 100%;
}

::v-deep .warning {
  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;
}
</style>

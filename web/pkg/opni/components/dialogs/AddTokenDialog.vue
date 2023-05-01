<script>
import AsyncButton from '@shell/components/AsyncButton';
import { Card } from '@components/Card';
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import { Checkbox } from '@components/Form/Checkbox';
import Select from '@shell/components/form/Select';
import { createToken } from '../../utils/requests/management';
import { exceptionToErrorsArray } from '../../utils/error';

export default {
  components: {
    Card,
    AsyncButton,
    LabeledInput,
    LabeledSelect,
    Checkbox,
    Select
  },
  data() {
    const expirationOptions = [
      {
        label: '10 min',
        value: '600s'
      },
      {
        label: '30 min',
        value: '1800s'
      },
      {
        label: '1 day',
        value: '86400s'
      },
      {
        label: '7 days',
        value: '604800s'
      },
      {
        label: 'custom',
        value: 'custom'
      }
    ];
    const customExpirationUnitsOptions = [
      {
        label: 'minutes',
        value: 60
      },
      {
        label: 'hours',
        value: 3600
      },
      {
        label: 'days',
        value: 86400
      },
      {
        label: 'weeks',
        value: 604800
      },
    ];

    return {
      name:                  null,
      expirationOptions,
      expiration:            expirationOptions[0],
      customExpirationUnit:  customExpirationUnitsOptions[0].value,
      customExpirationUnitsOptions,
      customExpirationValue: 1,
      clusters:              [],
      capabilities:          {
        join: {
          enabled: false,
          cluster: '',
        },
      },
    };
  },
  methods: {
    close() {
      this.$modal.hide('add-token-dialog');
    },

    open(clusters) {
      this.$set(this, 'clusters', clusters.map(cluster => ({
        label: cluster.nameDisplay,
        value: cluster.id,
      })));
      this.$set(this, 'expiration', this.expirationOptions[0].value);
      this.$modal.show('add-token-dialog');
    },

    async apply(buttonDone) {
      try {
        let expiration = this.expiration;

        if (expiration === 'custom') {
          const unitMultiplier = this.customExpirationUnit;
          const seconds = Math.min(
            Math.max(1, this.customExpirationValue) * unitMultiplier,
            365 * 24 * 60 * 60);

          expiration = `${ seconds }s`;
        }
        const caps = [];

        if (this.capabilities.join.enabled) {
          caps.push({
            type:      'join_existing_cluster',
            reference: { id: this.capabilities.join.cluster }
          });
        }
        await createToken(expiration, this.name || undefined, caps);
        buttonDone(true);
        this.$emit('save');
        this.close();
      } catch (err) {
        this.errors = exceptionToErrorsArray(err);
        buttonDone(false);
      }
    }
  }
};
</script>

<template>
  <modal
    class="add-token-dialog"
    name="add-token-dialog"
    styles="background-color: var(--nav-bg); border-radius: var(--border-radius); max-height: 100vh;"
    height="auto"
    :width="600"
    :scrollable="true"
    @closed="close()"
  >
    <Card class="prompt-restore" :show-highlight-border="false" title="Create Token">
      <h2 slot="title" class="text-default-text" v-html="'Create Token'" />
      <div slot="body">
        <div class="pt-10 pb-10">
          <div class="row mb-10">
            <div class="col span-12">
              <LabeledInput v-model.trim="name" label="Name (optional)" />
            </div>
          </div>
          <div class="row">
            <div class="col span-12">
              <LabeledSelect
                v-model="expiration"
                label="Expiration"
                :options="expirationOptions"
              />
            </div>
          </div>
          <div v-if="expiration === 'custom'" class="row span-6 mt-5 expiry">
            <input
              v-model="customExpirationValue"
              type="number"
              min="1"
              :mode="'edit'"
            >
            <Select
              v-model="customExpirationUnit"
              class="ml-10"
              :options="customExpirationUnitsOptions"
              :clearable="false"
            />
          </div>
        </div>
        <hr />
        <div class="row pt-10">
          <div class="col span-12">
            <h3 class="text-default-text">
              Extended Capabilities <span class="text-muted">(optional)</span>
            </h3>
            <div class="option-row row pt-10">
              <div class="col span-4">
                <Checkbox
                  v-model="capabilities.join.enabled"
                  class="join-checkbox"
                  label="Join Existing Cluster"
                  tooltip="Allow using this token to add new capabilities to an existing cluster."
                  :disabled="clusters.length === 0"
                />
                <p
                  v-if="!capabilities.join.enabled && clusters.length === 0"
                  class="no-clusters-warn"
                >
                  No Clusters
                </p>
              </div>
              <div class="col span-8">
                <LabeledSelect
                  v-model="capabilities.join.cluster"
                  label="Cluster"
                  placeholder="Choose an existing cluster"
                  :options="clusters"
                  :disabled="!capabilities.join.enabled"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div slot="actions" class="buttons">
        <button class="btn role-secondary mr-10" @click="close">
          {{ t('generic.cancel') }}
        </button>

        <AsyncButton
          mode="create"
          @click="apply"
        />
      </div>
    </Card>
  </modal>
</template>
<style lang='scss' scoped>
  .prompt-restore {
    margin: 0;
  }
  .buttons {
    display: flex;
    justify-content: flex-end;
    width: 100%;
  }

  .add-token-dialog {
    border-radius: var(--border-radius);
    overflow: scroll;
    max-height: 100vh;
    & ::-webkit-scrollbar-corner {
      background: rgba(0,0,0,0);
    }
  }

  .option-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .join-checkbox + .no-clusters-warn {
    margin-top: 0;
    color: var(--warning);
    position: absolute; // don't push the checkbox up
    margin-left: 19px; // align with checkbox label
  }

  .expiry {
    display: flex;
    align-items: center;
  }
</style>

<script>
import LabeledSelect from '@shell/components/form/LabeledSelect';

export const GRANULARITIES = [
  {
    label: '1h', unit: 'h', count: 1
  },
  {
    label: '30m', unit: 'm', count: 30
  },
  {
    label: '10m', unit: 'm', count: 10
  },
];

export const RANGES = [
  {
    label: '24h', unit: 'h', count: 24
  },
  {
    label: '12h', unit: 'h', count: 12
  },
  {
    label: '1h', unit: 'h', count: 1
  }
];

export const REFRESH_RATES = [
  {
    label: '30s', unit: 's', count: 30
  },
  {
    label: '1m', unit: 'm', count: 1
  },
  {
    label: '30m', unit: 'm', count: 30
  },
];

export const DEFAULT_CONFIGURATION = {
  granularity: GRANULARITIES[0],
  range:       RANGES[0],
  refreshRate: REFRESH_RATES[2]
};

export default {
  components: { LabeledSelect },

  props: {
    value: {
      type:     Object,
      required: true
    },
  },

  data() {
    return {
      granularities:      GRANULARITIES,
      ranges:             RANGES,
      refreshRates:       REFRESH_RATES,
    };
  },

  methods: {
    update(prop, value) {
      this.value[prop] = value;
      this.$emit('input', this.value);
    }
  }
};
</script>
<template>
  <div class="row">
    <div class="col span-1">
      <LabeledSelect
        class="refresh"
        :label="t('opni.configurator.refresh')"
        :value="value.refreshRate"
        option-key="label"
        :options="refreshRates"
        placement="bottom"
        @input="update('refreshRate', $event)"
      />
    </div>
    <div class="col span-7"></div>
    <div class="col span-2">
      <LabeledSelect
        :label="t('opni.configurator.range')"
        :value="value.range"
        option-key="label"
        :options="ranges"
        placement="bottom"
        @input="update('range', $event)"
      />
    </div>
    <div class="col span-2">
      <LabeledSelect
        :label="t('opni.configurator.granularity')"
        :value="value.granularity"
        option-key="label"
        :options="granularities"
        placement="bottom"
        @input="update('granularity', $event)"
      />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.refresh {
  min-width: 100px;
}
</style>

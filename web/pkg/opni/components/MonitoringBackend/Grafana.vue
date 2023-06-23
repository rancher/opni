<script>
import { LabeledInput } from '@components/Form/LabeledInput';
import EnabledDisabled from '../EnabledDisabled';

export default {
  components: { EnabledDisabled, LabeledInput },

  props: {
    value: {
      type:     Object,
      required: true
    },

    status: {
      type:     Number,
      required: true
    }
  },
};
</script>
<template>
  <EnabledDisabled v-model="value.enabled" disabled-message="Grafana is not currently enabled. Enabling it will install additional resources.">
    <div class="row">
      <div class="col span-4">
        <LabeledInput v-model="value.hostname" label="Hostname" :required="true" />
      </div>
    </div>
  </EnabledDisabled>
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

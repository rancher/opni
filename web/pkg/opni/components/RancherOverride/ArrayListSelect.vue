<script>
import ArrayList from '@shell/components/form/ArrayList';
import Select from '@shell/components/form/Select';

export default {
  // eslint-disable-next-line vue/no-reserved-component-names
  components: { ArrayList, Select },
  props:      {
    value: {
      type:     Array,
      required: true
    },
    options: {
      default: null,
      type:    Array
    },
    selectProps: {
      type:    Object,
      default: null,
    },
    arrayListProps: {
      type:    Object,
      default: null
    },
    loading: {
      type:    Boolean,
      default: false
    },
    addLabel: {
      type:    String,
      default: null
    }
  },
  computed: {
    filteredOptions() {
      return this.options
        .filter(option => !this.value.includes(option.value));
    },

    addAllowed() {
      return this.filteredOptions.length > 0;
    }
  },

  methods: {
    updateRow(index, value) {
      this.value.splice(index, 1, value);
      this.$emit('input', this.value);
    },
    calculateOptions(value) {
      const valueOption = this.options.find(o => o.value === value);

      if (valueOption) {
        return [valueOption, ...this.filteredOptions];
      }

      return this.filteredOptions;
    }
  }
};
</script>

<template>
  <ArrayList
    v-bind="arrayListProps"
    :value="value"
    class="array-list-select"
    :add-allowed="addAllowed || loading"
    :loading="loading"
    :add-label="addLabel"
    @input="$emit('input', $event)"
  >
    <template #columns="scope">
      <Select
        :value="scope.row.value"
        v-bind="selectProps"
        :options="calculateOptions(scope.row.value)"
        @input="updateRow(scope.i, $event)"
      />
    </template>
  </ArrayList>
</template>

<style lang="scss" scoped>
::v-deep .unlabeled-select {
    height: 61px;
}
</style>

<script>
import { Card } from '@components/Card';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import ArrayListSelect from '@shell/components/form/ArrayListSelect';

export default {
  components: {
    Card, LabeledSelect, ArrayListSelect
  },
  props:      {
    options: {
      type:     Array,
      required: true
    },

    value: {
      type:     Array,
      required: true
    },

    disabled: {
      type:    Boolean,
      default: false
    },

    taggable: {
      type:    Boolean,
      default: false
    },
  },

  methods: {
    filteredOptions(selectedKey) {
      const selectedKeys = this.value.map(v => v.key);

      return this.options.filter(o => o.key === selectedKey || !selectedKeys.includes(o.key));
    },

    filteredKeyOptions(selectedKey) {
      return this.filteredOptions(selectedKey).map(o => o.key);
    },

    filteredValueOptions(selectedKey) {
      const out = this.options.find(o => o.key === selectedKey)?.vals || [];

      return out;
    },

    addFilter() {
      this.$emit('input', [...this.value, { key: '', vals: [] }]);
    },

    removeFilter(i) {
      const val = [...this.value];

      val.splice(i, 1);
      this.$emit('input', val);
    }
  },

  watch: {}
};
</script>
<template>
  <div class="metric-filter">
    <h5>Metric Filter</h5>
    <Card v-for="(filter, i) in value" :key="i" class="ml-0 mr-0" :show-highlight-border="false" :show-actions="false">
      <div slot="body" class="card-body">
        <div class="row mt-25">
          <div class="col span-6">
            <LabeledSelect v-model="filter.key" label="Event Key" :options="filteredKeyOptions(filter.key)" :disabled="disabled" />
          </div>
          <div class="col span-6 middle">
            <ArrayListSelect v-model="filter.vals" :options="filteredValueOptions(filter.key)" add-label="Add Event Value" :disabled="disabled" :taggable="taggable" />
          </div>
        </div>
        <button class="close btn role-link close btn-sm" :disabled="disabled" @click="() => removeFilter(i)">
          <i class="icon icon-x" />
        </button>
      </div>
    </Card>
    <div>
      <button class="btn role-tertiary" :disabled="disabled || value.length === options.length" @click="addFilter">
        Add Filter
      </button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
  ::v-deep {
    .card-container {
      min-height: initial;
    }
    .card-body {
      position: relative;

      .close {
        position: absolute;
        right: 0;
        top: 0;

        font-size: 1.5em;
      }

      .middle {
        display: flex;
        flex-direction: column;
        justify-content: center;
      }
    }
  }
</style>

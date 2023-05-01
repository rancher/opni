<script>
import SortableTable from '@shell/components/SortableTable';
import Loading from '@shell/components/Loading';
import { getClusterStatus, getAlertEndpoints } from '../utils/requests/alerts';

export default {
  components: { Loading, SortableTable },
  async fetch() {
    await this.load();
  },

  data() {
    return {
      loading:             false,
      statsInterval:       null,
      endpoints:           [],
      isAlertingEnabled: false,
      headers:             [
        {
          name:            'nameDisplay',
          labelKey:        'tableHeaders.name',
          value:           'nameDisplay',
        },
        {
          name:            'type',
          labelKey:        'tableHeaders.type',
          value:           'typeDisplay',
        },
        {
          name:            'description',
          labelKey:        'tableHeaders.description',
          value:           'description'
        },
      ]
    };
  },

  created() {
    this.$on('remove', this.onRemove);
    this.$on('clone', this.onClone);
    this.statsInterval = setInterval(this.updateStatuses, 10000);
  },

  beforeDestroy() {
    this.$off('remove');
    this.$off('clone');
    if (this.statsInterval) {
      clearInterval(this.statsInterval);
    }
  },

  methods: {
    onRemove() {
      this.load();
    },

    onClone() {
      this.load();
    },

    async load() {
      try {
        this.loading = true;

        const status = (await getClusterStatus()).state;
        const isAlertingEnabled = status === 'Installed';

        this.$set(this, 'isAlertingEnabled', isAlertingEnabled);

        if (isAlertingEnabled) {
          this.$set(this, 'endpoints', await getAlertEndpoints(this));
        }
      } finally {
        this.loading = false;
      }
    },
  },
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <div class="title">
        <h1>Endpoints</h1>
      </div>
      <div v-if="isAlertingEnabled" class="actions-container">
        <n-link class="btn role-primary" :to="{name: 'endpoint-create'}">
          Create
        </n-link>
      </div>
    </header>
    <SortableTable
      v-if="isAlertingEnabled"
      :rows="endpoints"
      :headers="headers"
      :search="false"
      default-sort-by="expirationDate"
      key-field="id"
    />
    <div v-else class="not-enabled">
      <h4>
        Alerting must be enabled to use Endpoints. <n-link :to="{name: 'alerting-backend'}">
          Click here
        </n-link> to enable alerting.
      </h4>
    </div>
  </div>
</template>

<style lang="scss" scoped>
::v-deep {
  .nowrap {
    white-space: nowrap;
  }

  .monospace {
    font-family: $mono-font;
  }
}

.not-enabled {
    text-align: center;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    height: 100%;
  }
</style>

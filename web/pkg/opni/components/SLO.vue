<script>
import { mapGetters } from 'vuex';
import { LabeledInput } from '@components/Form/LabeledInput';
import LabeledSelect from '@shell/components/form/LabeledSelect';
import ArrayList from '@shell/components/form/ArrayList';
import AsyncButton from '@shell/components/AsyncButton';
import Loading from '@shell/components/Loading';
import { Banner } from '@components/Banner';
import Tab from '@shell/components/Tabbed/Tab';
import Tabbed from '@shell/components/Tabbed';
import UnitInput from '@shell/components/form/UnitInput';
import { exceptionToErrorsArray } from '../utils/error';
import {
  createSLO, getMetrics, getServices, updateSLO, getEvents, getSLO
} from '../utils/requests/slo';
import MetricFilter from './MetricFilter';
import SLOPreview from './SLOPreview';
import AttachedEndpoints, { createDefaultAttachedEndpoints } from './AttachedEndpoints';

export default {
  components: {
    AsyncButton,
    AttachedEndpoints,
    LabeledInput,
    LabeledSelect,
    ArrayList,
    Loading,
    Banner,
    Tab,
    Tabbed,
    UnitInput,
    MetricFilter,
    SLOPreview
  },

  async fetch() {
    const sloRequest = this.$route.params.id ? getSLO(this.$route.params.id, this) : Promise.resolve(false);

    if (await sloRequest) {
      const slo = await sloRequest;

      this.$set(this, 'cluster', slo.clusterId);
      await this.loadServices();

      this.$set(this, 'service', slo.serviceId);
      await this.loadMetrics();

      this.$set(this, 'goodMetric', slo.goodMetricName);
      this.$set(this, 'totalMetric', slo.totalMetricName);
      await Promise.all([this.loadGoodEvents(), this.loadTotalEvents()]);

      this.$set(this, 'id', slo.id);
      this.$set(this, 'name', slo.name);
      this.$set(this, 'period', slo.period);
      this.$set(this, 'tags', slo.tags);
      this.$set(this, 'threshold', slo.threshold);
      this.$set(this, 'budgetingInterval', slo.budgetingInterval);
      this.$set(this, 'goodEvents', slo.goodEvents);
      this.$set(this, 'totalEvents', slo.totalEvents);
      const defaultAttachedEndpoints = createDefaultAttachedEndpoints();

      this.$set(this, 'attachedEndpoints', {
        ...defaultAttachedEndpoints,
        details: { ...defaultAttachedEndpoints.details, ...slo.attachedEndpoints.details },
        ...slo.attachedEndpoints
      });
    }
  },

  data() {
    return {
      id:                       '',
      name:                     '',
      loadingMetrics:           false,

      cluster: null,

      service:                  null,
      servicesLoading:          false,
      serviceOptions:           [],

      goodMetric:               null,
      totalMetric:              null,
      metricLoading:            false,
      metricOptions:            [],

      goodEventOptions:         [],
      goodEvents:               [],

      totalEventOptions:        [],
      totalEvents:              [],
      threshold:                95,

      rawMetrics:               [],
      periodOptions:            ['7d', '28d', '30d'],
      period:                   '7d',

      tags:                     [],
      metricName:               '',
      error:                    '',
      budgetingIntervalOptions: ['1m', '5m', '10m', '30m', '60m'],
      budgetingInterval:        '5m',

      attachedEndpoints: createDefaultAttachedEndpoints(),

    };
  },

  methods: {
    async save(buttonCallback) {
      if (this.name === '') {
        this.$set(this, 'error', 'Name is required');
        buttonCallback(false);

        return;
      }

      if (this.service === '') {
        this.$set(this, 'error', 'A Service is required');
        buttonCallback(false);

        return;
      }

      if (this.goodMetric === '') {
        this.$set(this, 'error', 'A Positive Count is required');
        buttonCallback(false);

        return;
      }

      if (this.goodEvents.length === 0) {
        this.$set(this, 'error', 'A Positive Count Metric Filter is required');
        buttonCallback(false);

        return;
      }

      if (this.totalMetric === '') {
        this.$set(this, 'error', 'A Total Count is required');
        buttonCallback(false);

        return;
      }

      if (this.period === '') {
        this.$set(this, 'error', 'Period is required');
        buttonCallback(false);

        return;
      }

      if (this.budgetingInterval === '') {
        this.$set(this, 'error', 'Budgeting Interval is required');
        buttonCallback(false);

        return;
      }

      try {
        const hasAttachedEndpoints = this.attachedEndpoints.items.length > 0 && this.attachedEndpoints.items.some(item => item?.endpointId);
        const attachedEndpoints = hasAttachedEndpoints ? this.attachedEndpoints : undefined;

        if (this.id) {
          await updateSLO(this.id, this.name, this.cluster, this.service, this.goodMetric, this.totalMetric, this.goodEvents, this.totalEvents, this.period, this.budgetingInterval, this.threshold, this.tags, attachedEndpoints);
        } else {
          await createSLO(this.name, this.cluster, this.service, this.goodMetric, this.totalMetric, this.goodEvents, this.totalEvents, this.period, this.budgetingInterval, this.threshold, this.tags, attachedEndpoints);
        }

        this.$set(this, 'error', '');
        buttonCallback(true);
        this.$router.replace({ name: 'slos' });
      } catch (err) {
        this.$set(this, 'error', exceptionToErrorsArray(err).join('; '));
        buttonCallback(false);
      }
    },
    async loadServices() {
      this.$set(this, 'service', null);
      this.$set(this, 'servicesLoading', true);
      const servicesRequest = getServices(this.cluster);

      const services = await servicesRequest;

      this.$set(this, 'serviceOptions', services.map(s => ({ label: s.id, value: s.id })));
      this.$set(this, 'servicesLoading', false);
    },
    async loadMetrics() {
      this.$set(this, 'metricLoading', true);
      this.$set(this, 'goodMetric', null);
      this.$set(this, 'totalMetric', null);

      const metricsRequest = getMetrics(this.cluster, this.service);
      const metrics = await metricsRequest;

      const options = [];

      Object.entries(metrics.groupNameToMetrics).forEach(([key, value], i) => {
        options.push({
          kind: 'group', label: key, value: `group${ key }`
        });
        options.push({
          kind: 'divider', label: `divider${ key }`, value: `divider${ key }`
        });

        value.items.forEach((item) => {
          options.push({ label: item.id, value: item.id });
        });
      });

      this.$set(this, 'metricOptions', options);
      this.$set(this, 'metricLoading', false);
    },

    async loadGoodEvents() {
      const eventsRequest = getEvents(this.cluster, this.service, this.goodMetric);
      const events = await eventsRequest;

      this.$set(this, 'goodEventOptions', events);
      this.$set(this, 'goodEvents', []);
    },

    async loadTotalEvents() {
      const eventsRequest = getEvents(this.cluster, this.service, this.totalMetric);
      const events = await eventsRequest;

      this.$set(this, 'totalEventOptions', events);
      this.$set(this, 'totalEvents', []);
    },

    cancel() {
      this.$router.replace({ name: 'slos' });
    }
  },

  computed: {
    ...mapGetters({ clusters: 'opni/clusters' }),

    clusterOptions() {
      return this.clusters.map(c => ({ label: c.nameDisplay, value: c.id }));
    },

    isEdit() {
      return !!this.$route.params.id;
    },
    metric() {
      return this.rawMetrics.find(metric => metric.name === this.metricName) || {};
    },
    mismatchTotal() {
      return this.totalMetric && this.totalMetric !== this.goodMetric;
    },
    totalStatus() {
      return this.mismatchTotal ? 'warning' : null;
    },

    totalTooltip() {
      return this.mismatchTotal ? `In most cases you'll want to use the same Total Count Metric as the Positive Count Metric. Only change this if you're confident in what you're doing.` : null;
    }
  },

  watch: {
    cluster() {
      if (!this.$fetchState.pending) {
        this.loadServices();
      }
    },

    service() {
      if (!this.$fetchState.pending) {
        this.loadMetrics();
      }
    },

    goodMetric() {
      if (!this.$fetchState.pending) {
        this.loadGoodEvents();
        if (this.totalMetric === '' || this.totalMetric === null) {
          this.$set(this, 'totalMetric', this.goodMetric);
        }
      }
    },

    totalMetric() {
      if (!this.$fetchState.pending) {
        this.loadTotalEvents();
      }
    }
  },
};
</script>
<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else-if="clusterOptions.length === 0" class="no-services">
    There aren't any clusters available to watch. Try adding a cluster.
  </div>
  <div v-else>
    <div class="row mb-20">
      <div class="col span-12">
        <LabeledInput
          v-model="name"
          label="Name"
          :required="true"
        />
      </div>
    </div>
    <Tabbed class="mb-20" :side-tabs="true">
      <Tab
        name="options"
        :label="'Options'"
        :weight="3"
      >
        <div class="row mb-20 bottom">
          <div class="col span-6">
            <LabeledSelect
              v-model="cluster"
              label="Cluster"
              :options="clusterOptions"
              :searchable="true"
            />
          </div>
          <div class="col span-6">
            <LabeledSelect
              v-model="service"
              label="Service"
              :options="serviceOptions"
              :searchable="true"
              :disabled="!cluster"
              :loading="servicesLoading"
            />
          </div>
        </div>
        <div class="row mb-0">
          <div class="col span-12">
            <h3>Positive Count</h3>
          </div>
        </div>
        <div class="row ">
          <div class="col span-6">
            <LabeledSelect
              v-model="goodMetric"
              label="Metric"
              :options="metricOptions"
              :searchable="true"
              :disabled="!service"
              :loading="metricLoading"
            />
          </div>
        </div>
        <div class="row bottom mb-20 mt-10">
          <div class="col span-12">
            <MetricFilter v-model="goodEvents" :options="goodEventOptions" :disabled="!goodMetric" :taggable="true" />
          </div>
        </div>
        <div class="row mb-0">
          <div class="col span-12">
            <h3>Total Count</h3>
          </div>
        </div>
        <div class="row">
          <div class="col span-6">
            <LabeledSelect
              v-model="totalMetric"
              label="Metric"
              :options="metricOptions"
              :searchable="true"
              :disabled="!service"
              :loading="metricLoading"
              :status="totalStatus"
              :tooltip="totalTooltip"
            />
          </div>
        </div>
        <div class="row bottom mb-20 mt-10">
          <div class="col span-12">
            <MetricFilter v-model="totalEvents" :options="totalEventOptions" :disabled="!totalMetric" />
          </div>
        </div>
        <div class="row">
          <div class="col span-12">
            <h3>Threshold</h3>
          </div>
        </div>
        <div class="row mb-20">
          <div class="col span-6">
            <div>
              <UnitInput
                v-model="threshold"
                label="Threshold"
                suffix="%"
              />
            </div>
          </div>
          <div class="col span-6">
            <LabeledSelect
              v-model="period"
              label="Period"
              :options="periodOptions"
            />
          </div>
        </div>
        <div class="row mb-20 bottom">
          <div class="col span-6">
            <LabeledSelect
              v-model="budgetingInterval"
              label="Budgeting Interval"
              :options="budgetingIntervalOptions"
            />
          </div>
        </div>
        <div class="row">
          <div class="col span-12">
            <h3>Preview</h3>
          </div>
        </div>
        <div class="row">
          <div class="col span-12 center">
            <SLOPreview
              :name="name"
              :cluster-id="cluster"
              :service-id="service"
              :good-metric-name="goodMetric"
              :total-metric-name="totalMetric"
              :good-events="goodEvents"
              :total-events="totalEvents"
              :period="period"
              :budgeting-interval="budgetingInterval"
              :target-value="threshold"
            />
          </div>
        </div>
      </Tab>
      <Tab
        name="messaging"
        label="Message Options"
        :weight="2"
      >
        <AttachedEndpoints v-model="attachedEndpoints" :show-severity="false" />
      </Tab>
      <Tab
        name="tags"
        :label="'Tags'"
        :weight="1"
      >
        <ArrayList
          v-model="tags"
          mode="edit"
          label="Tags"
        />
      </Tab>
    </Tabbed>
    <div class="resource-footer">
      <button class="btn role-secondary mr-10" @click="cancel">
        Cancel
      </button>
      <AsyncButton mode="edit" @click="save" />
    </div>
    <Banner
      v-if="error"
      color="error"
      :label="error"
    />
  </div>
</template>

<style lang="scss" scoped>
.bottom {
  border-bottom: 1px solid var(--header-border);
  padding-bottom: 20px;
}

.middle {
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.resource-footer {
  display: flex;
  flex-direction: row;

  justify-content: flex-end;
}

.no-services {
  width: 100%;
  height: 100%;

  display: flex;
  flex-direction: row;

  justify-content: center;
  align-items: center;

  font-size: 20px;
}

.center {
  text-align: center;

  img {
    width: 100%;
  }
}
</style>

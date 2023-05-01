<script>
import Loading from '@shell/components/Loading';
import dayjs from 'dayjs';
import { getAlertConditionsWithStatus, getConditionTimeline, getClusterStatus } from '../utils/requests/alerts';
import { getClusters } from '../utils/requests/management';

export default {
  components: { Loading },
  async fetch() {
    await this.load();
  },

  data() {
    return {
      loading:             false,
      conditions:          [],
      groups:            [],
      isAlertingEnabled: false,
      headers:             [
        {
          name:          'status',
          labelKey:      'tableHeaders.status',
          value:         'status',
          formatter:     'StatusBadge',
          width:     100
        },
        {
          name:          'nameDisplay',
          labelKey:      'tableHeaders.name',
          value:         'nameDisplay',
          width:         undefined
        },
        {
          name:          'tags',
          labelKey:      'tableHeaders.tags',
          value:         'tags',
          formatter:     'ListBubbles'
        },
        {
          name:      'period',
          labelKey:  'tableHeaders.period',
          value:     'period'
        },
      ]
    };
  },

  methods: {
    async load() {
      const status = (await getClusterStatus()).state;
      const isAlertingEnabled = status === 'Installed';

      this.$set(this, 'isAlertingEnabled', isAlertingEnabled);

      if (!isAlertingEnabled) {
        return;
      }

      const now = dayjs();
      const clusters = await getClusters(this);
      const [conditions, response] = await Promise.all([getAlertConditionsWithStatus(this, clusters), getConditionTimeline({ lookbackWindow: '24h' })]);

      const DEFAULT_CLUSTER_ID = 'default';
      const UPSTREAM_CLUSTER_ID = 'UPSTREAM_CLUSTER_ID';

      const timelines = Object.entries(response?.items || {})
        .map(([id, value]) => {
          const condition = conditions.find(c => c.id === id);

          if (!condition) {
            return { events: [] };
          }

          return {
            name:      condition.nameDisplay.replace(/\(.*\)/g, ''),
            clusterId: condition.clusterId || DEFAULT_CLUSTER_ID,
            events:    (value?.windows || [])
              .filter(w => w.type !== 'Timeline_Unknown')
              .map(w => ({
                start:       now.diff(dayjs(w.start), 'h', true),
                end:         now.diff(dayjs(w.end), 'h', true),
                type:        w.type
              }))
          };
        })
        .filter(t => t.events.length > 0);

      const groups = {
        [DEFAULT_CLUSTER_ID]: {
          name:      'Disconnected',
          timelines: []
        },
        [UPSTREAM_CLUSTER_ID]: {
          name:      'Upstream',
          timelines: []
        }
      };

      clusters.forEach((c) => {
        groups[c.id] = {
          name:      c.nameDisplay,
          timelines: []
        };
      });

      timelines.forEach((t) => {
        groups[t.clusterId].timelines.push(t);
      });

      Object.entries(groups).forEach(([key, value]) => {
        if (value.timelines.length === 0) {
          delete groups[key];
        }
      });

      this.$set(this, 'groups', groups);
    },

    computeEventLeft(event) {
      return `${ ((24 - event.start) * 100 / 26) + 4 }%`;
    },

    computeEventWidth(event) {
      return `${ (event.start - event.end) * 100 / 26 }%`;
    },

    computeTickLeft(i) {
      return `${ ((i - 1) * 100 / 13) + 4 }%`;
    },

    computeTooltip(event) {
      if (event.type === 'Timeline_Silenced') {
        return 'Silenced Event';
      }

      return 'Agent Capability Unhealthy';
    }
  },
  computed: {
    hasTimelines() {
      return Object.values(this.groups).some(g => g.timelines.length > 0);
    }
  }
};
</script>
<template>
  <Loading v-if="loading || $fetchState.pending" />
  <div v-else>
    <header>
      <div class="title">
        <h1>Overview</h1>
      </div>
    </header>
    <table v-if="isAlertingEnabled" class="sortable-table top-divider" width="100%">
      <thead class="sortable-table top-divider">
        <tr>
          <th>Incident</th>
          <th>24hrs</th>
          <th>22hrs</th>
          <th>20hrs</th>
          <th>18hrs</th>
          <th>16hrs</th>
          <th>14hrs</th>
          <th>12hrs</th>
          <th>10hrs</th>
          <th>8hrs</th>
          <th>6hrs</th>
          <th>4hrs</th>
          <th>2hrs</th>
          <th>0hrs</th>
        </tr>
      </thead>
      <tbody v-for="(group, i) in groups" :key="i" class="group">
        <tr :key="group.name" class="group-row">
          <td colspan="14">
            <div class="group-tab">
              <div class="cluster">
                Cluster: {{ group.name }}
              </div>
            </div>
          </td>
        </tr>
        <tr v-for="(timeline, j) in group.timelines" :key="j" class="main-row">
          <td>{{ timeline.name }}</td>
          <td colspan="13" class="events">
            <div v-for="k in 13" :key="'tick'+k" class="tick" :style="{left: computeTickLeft(k)}">
&nbsp;
            </div>
            <div
              v-for="(event, k) in timeline.events"
              :key="'event'+k"
              v-tooltip="computeTooltip(event)"
              class="event"
              :class="event.type"
              :style="{left: computeEventLeft(event), width: computeEventWidth(event), }"
            >
              &nbsp;
            </div>
          </td>
        </tr>
      </tbody>
      <tbody v-if="!hasTimelines">
        <tr class="no-data">
          <td colspan="14">
            No events have occured in the last 24 hours
          </td>
        </tr>
      </tbody>
    </table>
    <div v-else class="not-enabled">
      <h4>
        Alerting must be enabled to use Alerting Overview. <n-link :to="{name: 'alerting-backend'}">
          Click here
        </n-link> to enable Alerting.
      </h4>
    </div>
  </div>
</template>

<style lang="scss" scoped>
table {
  table-layout: fixed;
}

.sortable-table tbody tr:not(.group-row):hover {
  background-color: var(--sortable-table-row-bg);
}

td, th {
  &:first-of-type {
    width: 300px;
    text-align: left;
  }
}

.sortable-table tbody tr.group-row .group-tab {
  top: 3px;
}

.cluster {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  width: 250px;
  height: 100%;
}

th {
  padding: 14px 5px;
  font-weight: normal;
  border: 0;
  color: var(--body-text);
  background-color: var(--sortable-table-header-bg);
}

td {
  padding: 14px 5px;
}

tr.no-data {
  &, td {
    background-color: var(--sortable-table-row-bg);
    text-align: center;
  }
}

.timeline {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--sortable-table-top-divider);
}

.heading {
  background-color: var(--sortable-table-header-bg);
  border-bottom: 1px solid var(--sortable-table-top-divider);
}

.heading, .row {
  display: flex;
  flex-direction: row;
  width: 100%;
  padding: 14px;

  &>div:nth-of-type(1) {
    width: 300px;
  }

  &>div:nth-of-type(2) {
    width: 100%;
    display: flex;
    flex-direction: row;
    justify-content: space-evenly;
  }
}

.events {
  position: relative;

  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background-color: var(--sortable-table-top-divider);
  }
}

.row {
  &>div:nth-of-type(1) {
    border-right: 1px solid var(--sortable-table-top-divider);
  }

  &>div:nth-of-type(2) {
    position: relative;
  }
}

.event {
  background-color: var(--error);
  opacity: 0.75;

  &.Timeline_Silenced {
    background-color: var(--warning);
  }

  position: absolute;
  top: 8px;
  bottom: 8px;
  border-radius: var(--border-radius);

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

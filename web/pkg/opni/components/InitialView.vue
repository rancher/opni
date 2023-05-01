<script>
import { CATALOG } from '@shell/config/types';
import AsyncButton from '@shell/components/AsyncButton';
import Loading from '@shell/components/Loading';
import Wizard from '@shell/components/Wizard';

const REPO = 'https://github.com/rancher/opni.git';
const REPO_ANNOTATION = 'catalog.cattle.io/ui-source-repo';
const REPO_NAME = 'opni-repo';
const CHART_NAME = 'opni';
const DEPLOYED_STATE = 'deployed';

export default {
  components: {
    AsyncButton,
    Loading,
    Wizard
  },

  async fetch() {
    await this.loadControllerChart();
    await this.loadApps();

    if (!this.repo) {
      return;
    }

    if (this.isChartFullyInstalled) {
      this.$router.replace({ name: 'agents' });

      return;
    }

    this.initStepIndex = 1;
  },

  data() {
    const installSteps = [
      {
        name:  'repo',
        label: 'Install Opni Repository',
        ready: false,
      },
      {
        name:  'install',
        label: 'Install Opni Chart',
        ready: false,
      },
    ];

    return {
      allRepos:        null,
      controllerChart: null,
      kubewardenRepo:  null,
      install:         false,
      apps:            null,

      initStepIndex: 0,
      installSteps,
    };
  },

  computed: {
    isChartPartlyInstalled() {
      return !this.isChartFullyInstalled && this.apps?.length > 0;
    },

    isChartFullyInstalled() {
      if (location.search.includes('full=true')) {
        return true;
      }

      if (this.apps?.length !== 2) {
        return false;
      }

      return this.apps.every(app => app.status.summary.state === DEPLOYED_STATE);
    }
  },

  methods: {
    async addRepository(btnCb) {
      try {
        const repoObj = await this.$store.dispatch('management/create', {
          type:     CATALOG.CLUSTER_REPO,
          metadata: { name: REPO_NAME },
          spec:     { gitBranch: 'charts-repo', gitRepo: REPO },
        });

        await repoObj.save();
        this.repo = repoObj;

        await this.$refs.wizard.next();
        this.$set(this.$refs.wizard, 'activeStep', this.installSteps[1]);

        btnCb(true);
      } catch (e) {
        this.$store.dispatch('growl/fromError', e);
        btnCb(false);
      }
    },

    async loadApps() {
      if (!this.repo) {
        return null;
      }

      const apps = await this.$store.dispatch(`management/findAll`, { type: CATALOG.APP });

      this.apps = apps.filter(a => a?.spec?.chart?.metadata?.annotations[REPO_ANNOTATION] === this.repo.name);
    },

    async loadRepo() {
      const allRepos = await this.$store.dispatch(`management/findAll`, { type: CATALOG.CLUSTER_REPO });

      this.repo = allRepos?.find(r => r.spec.gitRepo === REPO);
    },

    async loadControllerChart() {
      await this.loadRepo();
      if ( !this.repo ) {
        return;
      }

      await this.$store.dispatch('catalog/load', { force: true });

      // Check to see that the chart we need are available
      const charts = this.$store.getters['catalog/rawCharts'];
      const chartValues = Object.values(charts);

      this.controllerChart = chartValues.find(
        chart => chart.chartName === CHART_NAME
      );
    },

    async chartRoute() {
      if ( !this.controllerChart ) {
        try {
          await this.loadControllerChart();
        } catch (e) {
          this.$store.dispatch('growl/fromError', e);

          return;
        }
      }

      this.controllerChart.goToInstall(CHART_NAME, 'local');
    },

    goToApps() {
      this.$router.replace({
        name:   'c-cluster-product-resource',
        params: {
          cluster: 'local', product: 'apps', resource: 'catalog.cattle.io.app'
        }
      });
    }
  }
};
</script>

<template>
  <Loading v-if="$fetchState.pending" />
  <div v-else class="container">
    <div v-if="!repo && !controllerChart && !install" class="title p-10 center">
      <h1 class="mb-20">
        <img
          src="../assets/images/opni-icon.svg"
          height="64"
        />
        {{ t("opni.title") }}
      </h1>
      <div class="description">
        {{ t("opni.description") }}
      </div>
      <button class="btn role-primary mt-40" @click="install = true">
        {{ t("opni.install") }}
      </button>
    </div>

    <Wizard
      v-else
      ref="wizard"
      :init-step-index="initStepIndex"
      :steps="installSteps"
      banner-title="Opni"
      banner-title-subtext="Install"
    >
      <template #bannerTitleImage>
        <img
          src="../assets/images/opni-icon.svg"
          height="64"
        />
      </template>
      <template #repo>
        <div class="mt-20 mb-20 center">
          <h2>
            Opni Repository Install
          </h2>
          <p>
            Install the Opni Repository by clicking the button below.
          </p>
          <AsyncButton
            class="mt-40"
            mode="edit"
            action-label="Install Repository"
            waiting-label="Installing"
            success-label="Installed"
            error-label="Installation Failed"
            @click="addRepository"
          />
        </div>
      </template>
      <template #controlsContainer>
        &nbsp;
      </template>
      <template #install>
        <template v-if="!repo">
          <div class="center">
            <h2 class="mt-20 mb-10">
              {{ t("opni.title") }}
            </h2>
            <div class="mt-20 mb-20 center">
              <p class="mb-20">
                {{ t("opni.description") }}
              </p>

              <AsyncButton
                class="mt-40"
                mode="edit"
                action-label="Install Repository"
                waiting-label="Installing"
                success-label="Installed"
                @click="addRepository"
              />
            </div>
          </div>
        </template>

        <template v-else>
          <div class="mt-20 mb-20 center">
            <h2>
              Opni Chart Install
            </h2>
            <div v-if="isChartPartlyInstalled">
              <p>
                It looks like the Opni chart installation has already begun but hasn't finished yet.<br />You can take a look at the status of the installation <a href="#" @click.prevent="goToApps">here</a>.
              </p>
            </div>
            <div v-else>
              <p>
                Install the Opni chart by clicking the button below.
              </p>
              <button
                class="btn role-primary mt-40"
                @click.prevent="chartRoute"
              >
                {{ t("opni.installChart") }}
              </button>
            </div>
          </div>
        </template>
      </template>
    </Wizard>
  </div>
</template>

<style lang="scss" scoped>
h1 {
  display: flex;
  flex-direction: row;

  justify-content: center;
  align-items: center;
}

.description {
  max-width: 800px;
  margin: 0 auto;
}

.center {
  text-align: center;
}
</style>

<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import WorkloadModelConfig from '../components/WorkloadModelConfig';

export default {
  components: { WorkloadModelConfig, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Workload' };
  }
};
</script>
<template>
  <Main>
    <WorkloadModelConfig />
  </Main>
</template>

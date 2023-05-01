<script>
import Main from '../components/Main';
import Cluster from '../components/Cluster';
import { choosePageLayout } from '../utils/standalone';

export default {
  components: { Cluster, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Cluster' };
  }
};
</script>
<template>
  <Main>
    <Cluster />
  </Main>
</template>

<script>
import Main from '../components/Main';
import Clusters from '../components/Clusters';
import { choosePageLayout } from '../utils/standalone';

export default {
  components: { Clusters, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Agents' };
  }
};
</script>
<template>
  <Main>
    <Clusters />
  </Main>
</template>

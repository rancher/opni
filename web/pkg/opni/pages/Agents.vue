<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import AgentsStandalone from '../components/AgentsStandalone';

export default {
  components: { AgentsStandalone, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Agents' };
  }
};
</script>
<template>
  <Main>
    <AgentsStandalone />
    <!-- <AgentsExtension v-else /> -->
  </Main>
</template>

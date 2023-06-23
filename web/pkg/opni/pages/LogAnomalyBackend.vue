<script>
import Main from '../components/Main';
import LogAnomalyBackend from '../components/LogAnomalyBackend';
import { choosePageLayout } from '../utils/standalone';

export default {
  components: { LogAnomalyBackend, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Log Anomaly' };
  }
};
</script>
<template>
  <Main>
    <LogAnomalyBackend />
  </Main>
</template>

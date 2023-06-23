<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import MonitoringBackend from '../components/MonitoringBackend';

export default {
  components: { MonitoringBackend, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Monitoring' };
  }
};
</script>
<template>
  <Main>
    <MonitoringBackend />
  </Main>
</template>

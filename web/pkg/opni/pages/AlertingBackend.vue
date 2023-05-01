<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import AlertingBackend from '../components/AlertingBackend';

export default {
  components: { AlertingBackend, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Alerting' };
  }
};
</script>
<template>
  <Main>
    <AlertingBackend />
  </Main>
</template>

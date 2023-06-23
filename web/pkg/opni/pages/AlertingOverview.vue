<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import AlertingOverview from '../components/AlertingOverview';

export default {
  components: { AlertingOverview, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Alerting Overview' };
  }
};
</script>
<template>
  <Main>
    <AlertingOverview />
  </Main>
</template>

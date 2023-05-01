<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import SLO from '../components/SLO';

export default {
  components: { SLO, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - SLO' };
  }
};
</script>
<template>
  <Main>
    <SLO />
  </Main>
</template>

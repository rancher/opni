<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import SLOs from '../components/SLOs';

export default {
  components: { SLOs, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - SLOs' };
  }
};
</script>
<template>
  <Main>
    <SLOs />
  </Main>
</template>

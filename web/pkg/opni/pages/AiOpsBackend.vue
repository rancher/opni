<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import AiOpsBackend from '../components/AiOpsBackend';

export default {
  components: { AiOpsBackend, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - AI Ops' };
  }
};
</script>
<template>
  <Main>
    <AiOpsBackend />
  </Main>
</template>

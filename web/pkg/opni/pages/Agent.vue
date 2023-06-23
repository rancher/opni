<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Agent from '../components/Agent';

export default {
  components: { Agent, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Agent' };
  }
};
</script>
<template>
  <Main>
    <Agent />
  </Main>
</template>

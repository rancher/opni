<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Configuration from '../components/Configuration';

export default {
  components: { Configuration, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Configuration' };
  }
};
</script>
<template>
  <Main>
    <Configuration />
  </Main>
</template>

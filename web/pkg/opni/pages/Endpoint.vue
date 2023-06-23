<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Endpoint from '../components/Endpoint';

export default {
  components: { Endpoint, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Endpoint' };
  }
};
</script>
<template>
  <Main>
    <Endpoint />
  </Main>
</template>

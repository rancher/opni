<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Endpoints from '../components/Endpoints';

export default {
  components: { Endpoints, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Endpoints' };
  }
};
</script>
<template>
  <Main>
    <Endpoints />
  </Main>
</template>

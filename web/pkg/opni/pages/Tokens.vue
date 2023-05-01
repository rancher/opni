<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Tokens from '../components/Tokens';

export default {
  components: { Tokens, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Tokens' };
  }
};
</script>
<template>
  <Main>
    <Tokens />
  </Main>
</template>

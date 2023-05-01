<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Role from '../components/Role';

export default {
  components: { Role, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Role' };
  }
};
</script>
<template>
  <Main>
    <Role />
  </Main>
</template>

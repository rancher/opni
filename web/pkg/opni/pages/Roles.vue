<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Roles from '../components/Roles';

export default {
  components: { Roles, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Roles' };
  }
};
</script>
<template>
  <Main>
    <Roles />
  </Main>
</template>

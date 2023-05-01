<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import RBAC from '../components/RBAC';

export default {
  components: { RBAC, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - RBAC' };
  }
};
</script>
<template>
  <Main>
    <RBAC />
  </Main>
</template>

<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import RoleBindings from '../components/RoleBindings';

export default {
  components: { RoleBindings, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Role Bindings' };
  }
};
</script>
<template>
  <Main>
    <RoleBindings />
  </Main>
</template>

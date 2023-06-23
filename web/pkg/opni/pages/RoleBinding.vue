<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import RoleBinding from '../components/RoleBinding';

export default {
  components: { RoleBinding, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Role Binding' };
  }
};
</script>
<template>
  <Main>
    <RoleBinding />
  </Main>
</template>

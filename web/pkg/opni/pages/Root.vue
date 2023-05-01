<script>
import Root from '../components/Root';
import { choosePageLayout } from '../utils/standalone';

export default {
  components: { Root },
  layout:     choosePageLayout(),
  created() {
    if (process.env.isStandalone) {
      this.$router.push({ name: 'agents' });
    }
  },
  head() {
    return { title: 'Opni' };
  }
};
</script>
<template>
  <Root />
</template>

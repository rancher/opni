<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Snapshot from '../components/Snapshot';

export default {
  components: { Snapshot, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Snapshot' };
  }
};
</script>
<template>
  <Main>
    <Snapshot />
  </Main>
</template>

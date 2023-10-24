<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Snapshots from '../components/Snapshots';

export default {
  components: { Snapshots, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Snapshots' };
  }
};
</script>
<template>
  <Main>
    <Snapshots />
  </Main>
</template>

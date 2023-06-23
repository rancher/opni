<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Alarm from '../components/Alarm';

export default {
  components: { Alarm, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Alarm' };
  }
};
</script>
<template>
  <Main>
    <Alarm />
  </Main>
</template>

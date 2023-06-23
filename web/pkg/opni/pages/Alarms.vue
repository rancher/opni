<script>
import Main from '../components/Main';
import { choosePageLayout } from '../utils/standalone';
import Alarms from '../components/Alarms';

export default {
  components: { Alarms, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Alarms' };
  }
};
</script>
<template>
  <Main>
    <Alarms />
  </Main>
</template>

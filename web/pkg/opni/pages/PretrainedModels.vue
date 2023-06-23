<script>
import Main from '../components/Main';
import PretrainedModels from '../components/PretrainedModels';
import { choosePageLayout } from '../utils/standalone';

export default {
  components: { PretrainedModels, Main },
  layout:     choosePageLayout(),
  head() {
    return { title: 'Opni - Pretrained Models' };
  }
};
</script>
<template>
  <Main>
    <PretrainedModels />
  </Main>
</template>

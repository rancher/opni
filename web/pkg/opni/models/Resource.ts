import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';

export class Resource {
  protected vue: any;

  constructor(vue: any) {
    this.vue = vue;
  }

  currentRoute() {
    return {};
  }

  public promptRemove(resources = this) {
    GlobalEventBus.$emit('promptRemove', resources);
  }

  public promptEdit(resource = this) {
    GlobalEventBus.$emit('edit', resource);
  }

  public copy(resource = this) {
    GlobalEventBus.$emit('copy', resource);
  }

  public remove() {
    GlobalEventBus.$emit('remove');
  }

  public currentRouter() {
    return {};
  }
}

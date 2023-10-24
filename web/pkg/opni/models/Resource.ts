import GlobalEventBus from '@pkg/opni/utils/GlobalEventBus';

export class Resource {
  protected vue: any;

  constructor(vue: any) {
    this.vue = vue;
  }

  currentRoute() {
    return {};
  }

  public get id(): string {
    throw new Error('Not implemented');
  }

  public promptRemove(resources = this) {
    GlobalEventBus.$emit('promptRemove', resources);
  }

  public promptEdit(resource = this) {
    GlobalEventBus.$emit('promptEdit', resource);
  }

  public changeRoute(route: any) {
    if (!route) {
      throw new Error('A route needs to be provided');
    }

    GlobalEventBus.$emit('changeRoute', route);
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

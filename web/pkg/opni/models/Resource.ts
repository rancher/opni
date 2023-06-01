export class Resource {
  protected vue: any;

  constructor(vue: any) {
    this.vue = vue;
  }

  currentRoute() {
    return {};
  }

  public promptRemove(resources = this) {
    this.vue.$store.commit('action-menu/togglePromptRemove', resources, { root: true });
  }

  public promptEdit(resource = this) {
    this.vue.$emit('edit', resource);
  }

  public copy(resource = this) {
    this.vue.$emit('copy', resource);
  }

  public remove() {
    this.vue.$emit('remove');
  }

  public currentRouter() {
    return {};
  }
}

<template>
  <nav>
    <slot />
  </nav>
</template>
<style lang="scss" scoped>
nav {
    display: flex;
    flex-direction: column;
    position: relative;
    background-color: var(--nav-bg);
    border-right: var(--nav-border-size) solid var(--nav-border);
    overflow-y: auto;
}
</style>

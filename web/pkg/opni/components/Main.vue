<script>
import ActionMenu from '@shell/components/ActionMenu';
import PromptRemove from '@shell/components/PromptRemove';
import { createNavItemsFromNavigation } from '../utils/navigation';
import { isStandalone } from '../utils/standalone';
import { NAVIGATION } from '../router';
import SideNavColumn from './Navigation/SideNavColumn';
import SideNavColumnItems from './Navigation/SideNavColumn/Items';
import SideNavColumnItem from './Navigation/SideNavColumn/Item';
import HeaderBar from './Navigation/HeaderBar';

export default {
  components: {
    HeaderBar, SideNavColumn, SideNavColumnItems, SideNavColumnItem, ActionMenu, PromptRemove,
  },

  async fetch() {
    await this.load();
  },

  data() {
    return {
      // Assume home pages have routes where the name is the key to use for string lookup
      name:          this.$route.name,
      allNavItems:   [],
    };
  },

  methods: {
    async load() {
      const allNavItems = await createNavItemsFromNavigation(NAVIGATION, this.t.bind(this));

      this.$set(this, 'allNavItems', allNavItems);
    },

    isStandalone
  },

  computed: {
    configuration() {
      return this.allNavItems[this.allNavItems.length - 1];
    },

    navItems() {
      return this.allNavItems.slice(0, -1);
    },
  }

};
</script>

<template>
  <div class="opni-roots" :class="{standalone: isStandalone()}">
    <div class="opni-root">
      <HeaderBar v-if="isStandalone()" :simple="true">
        <div class="simple-title">
          <img :src="require('../assets/images/opni.svg')" />
        </div>
      </HeaderBar>

      <SideNavColumn>
        <SideNavColumnItems :items="navItems" />
        <SideNavColumnItem v-if="configuration" :item="configuration" />
      </SideNavColumn>
      <div class="opni-content p-20">
        <slot />
      </div>
      <ActionMenu />
      <PromptRemove />
    </div>
  </div>
  </div>
</template>

<style lang="scss">
  .dashboard-content .indented-panel {
    margin: 0;
    padding: 0;
    margin-top: -20px;
    height: calc(100% + 20px);
    width: initial;
  }

  .opni-roots {
    display: flex;
    flex-direction: column;
    height: 100%;

    &:not(.standalone) .opni-root {
      grid-template-areas:
        "nav main";
      grid-template-rows: auto;
    }
  }
  .opni-root {
    display: grid;
    flex: 1 1 auto;
    width: 100%;
    height: 100%;

    grid-template-areas:
      "header header"
      "nav main";

    grid-template-columns: var(--nav-width) auto;
    grid-template-rows: var(--header-height) auto var(--wm-height,0);

    > HEADER {
      grid-area: header;
    }

    > nav {
      grid-area: nav;
    }

    > .opni-content {
      grid-area: main;
    }
  }

  .simple-title img {
      object-fit: contain;
      height: 35px;
      max-width: 200px;
  }

  ::v-deep .main {
    grid-area: main;
    overflow: auto;

    display: flex;
    flex-direction: column;
    padding: 20px;
    min-height: 100%;

    .outlet {
      display: flex;
      flex-direction: column;
      padding: 20px;
      min-height: 100%;
    }

    FOOTER {
      background-color: var(--nav-bg);
      height: var(--footer-height);
    }

    HEADER {
      display: grid;
      grid-template-areas:  "type-banner type-banner"
                            "title actions"
                            "state-banner state-banner";
      grid-template-columns: auto auto;
      margin-bottom: 20px;
      align-content: center;
      min-height: 48px;

      .type-banner {
        grid-area: type-banner;
      }

      .state-banner {
        grid-area: state-banner;
      }

      .title {
        grid-area: title;
        align-self: center;
      }

      .actions-container {
        grid-area: actions;
        margin-left: 8px;
        align-self: center;
        text-align: right;
      }

      .role-multi-action {
        padding: 0 $input-padding-sm;
      }
    }
  }

  .opni-content {
    grid-area: main;
    overflow-y: scroll;

    display: flex;
    flex-direction: column;
    min-height: 100%;
  }

  .standalone .opni-content HEADER {
    width: 100%;
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: center;

    margin-bottom: 10px;
  }
</style>

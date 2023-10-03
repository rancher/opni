import { importTypes } from '@rancher/auto-import';
import { IPlugin } from '@shell/core/types';
import { axios } from '@pkg/opni/utils/axios';
import PromptRemove from '@shell/components/PromptRemove.vue';
import { watchClusters } from '@pkg/opni/utils/requests/management';
import { NAVIGATION } from './router.js';
import { flattenNavigation } from './utils/navigation';
import './styles/app.scss';
import { isStandalone } from './utils/standalone';
import store from './store';

// Init the package
export default function(plugin: IPlugin, context: any) {
  // Auto-import model, detail, edit from the folders
  importTypes(plugin);

  // Provide plugin metadata from package.json
  plugin.metadata = require('./package.json');

  if (isStandalone()) {
    // The backend requires that we pass application/json otherwise it will default to octet stream
    axios.defaults.headers.common['Accept'] = 'application/json';

    // We add this interceptor to prevent store requests from going through since we don't rely on the stores in standalone. It would probably be better if shell didn't make this request at all.
    context.$axios.interceptors.request.use((config: any) => {
      config.cancelToken = new axios.CancelToken(cancel => cancel('Not a valid request for standalone'));

      return config;
    }, (error: any) => {
      return Promise.reject(error);
    });

    // In standalone this method wouldn't work because we don't have an active product. This allows us to continue using the PromptRemove component in standalone.
    (PromptRemove as any).methods.refreshSpoofedTypes = () => {};
  }

  plugin.addProduct(require('./product'));
  plugin.addRoutes(flattenNavigation(NAVIGATION));
  plugin.addDashboardStore(store.config.namespace, store.specifics, store.config);

  watchClusters(context.app.store);
}

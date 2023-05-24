import { importTypes } from '@rancher/auto-import';
import { IPlugin } from '@shell/core/types';
import axios from 'axios';
import { NAVIGATION } from './router.js';
import { flattenNavigation } from './utils/navigation';
import './styles/app.scss';
import { isStandalone } from './utils/standalone';

// Init the package
export default function(plugin: IPlugin, context: any) {
  // Auto-import model, detail, edit from the folders
  importTypes(plugin);

  // Provide plugin metadata from package.json
  plugin.metadata = require('./package.json');

  // We add this interceptor to prevent store requests from going through since we don't rely on the stores in standalone. It would probably be better if shell didn't make this request at all.
  if (isStandalone()) {
    context.$axios.interceptors.request.use((config: any) => {
      config.cancelToken = new axios.CancelToken(cancel => cancel('Not a valid request for standalone'));

      return config;
    }, (error: any) => {
      return Promise.reject(error);
    });
  }

  plugin.addProduct(require('./product'));
  plugin.addRoutes(flattenNavigation(NAVIGATION));
}

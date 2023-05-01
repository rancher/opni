import { importTypes } from '@rancher/auto-import';
import { IPlugin } from '@shell/core/types';
import { NAVIGATION } from './router.js';
import { flattenNavigation } from './utils/navigation';
import './styles/themes/_light.scss';

// Init the package
export default function(plugin: IPlugin, context: any) {
  // Auto-import model, detail, edit from the folders
  importTypes(plugin);

  // Provide plugin metadata from package.json
  plugin.metadata = require('./package.json');

  plugin.addProduct(require('./product'));
  plugin.addRoutes(flattenNavigation(NAVIGATION));
}

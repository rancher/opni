import axios from 'axios';
import { isStandalone } from './utils/standalone';

export const NAME = 'Logging / Monitoring';
export function init(plugin: any, store: any) {
  const {
    product,
    // basicType,
    // configureType,
    // virtualType,
    // headers,
    // hideBulkActions,
  } = plugin.DSL(store, NAME);

  // We add this interceptor so we can redirect all opni requests through the k8s proxy when this is being used as an extension
  axios.interceptors.request.use((config: any) => {
    const prefix = '/opni-api/';

    if (!config.url.includes(prefix)) {
      return;
    }

    const clusterOverrideSearchParam = 'c=';
    const clusterId = window.location.search.includes(clusterOverrideSearchParam) ? window.location.search.replace(`?${ clusterOverrideSearchParam }`, '') : 'local';
    const isLocalCluster = clusterId === 'local';
    const clusterPrefix = isLocalCluster ? '' : `/k8s/clusters/${ clusterId }/`;
    const namespace = 'opni';
    const serviceName = 'opni-internal';
    const port = 11080;

    const proxy = isStandalone() ? '' : `${ clusterPrefix }api/v1/namespaces/${ namespace }/services/http:${ serviceName }:${ port }/proxy/`;

    config.url = `${ window.location.origin }${ proxy }${ config.url.replace(prefix, '') }`;

    return config;
  }, (error: any) => {
    return Promise.reject(error);
  });

  product({
    inStore:             'management',
    icon:                'file',
    label:               'Logging / Monitoring',
    removable:           false,
    showClusterSwitcher: false,
    category:            'global',
    to:                  { name: 'opni' }
  });
}

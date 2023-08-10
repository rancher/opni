// https://github.com/webpack/webpack/issues/13572#issuecomment-923736472
const crypto = require('crypto');
const cryptoOrigCreateHash = crypto.createHash;

crypto.createHash = algorithm => cryptoOrigCreateHash(algorithm === 'md4' ? 'sha256' : algorithm);

const config = require('@rancher/shell/vue.config');
const webpack = require('webpack');

const isStandalone = process.env.IS_STANDALONE === 'true';
let opniApi = process.env.OPNI_API || 'http://localhost:8888';

if (opniApi && !opniApi.startsWith('http')) {
  opniApi = `http://${ opniApi }`;
}

if (opniApi) {
  console.log(`OPNI API: ${ opniApi }`); // eslint-disable-line no-console
}

console.log(`IS STANDALONE`, isStandalone); // eslint-disable-line no-console

const baseConfig = config(__dirname, {
  excludes: [],
  // excludes: ['fleet', 'example']
});

const baseConfigureWebpack = baseConfig.configureWebpack;

baseConfig.devServer.proxy = {
  '/opni-api': {
    secure:      false,
    target:      opniApi,
    pathRewrite: { '^/opni-api': '' }
  },
};

baseConfig.configureWebpack = (config) => {
  config.cache = { type: 'filesystem' };

  config.plugins.push(new webpack.DefinePlugin({ 'process.env.isStandalone': JSON.stringify(isStandalone) }));

  baseConfigureWebpack(config);
};

// Makes the public path relative so that the <base> element will affect the assets.
if (!isStandalone) {
  baseConfig.publicPath = './';
}

// We need to add a custom script to the index in order to change how assets for the opni backendso we have to override the index.html
if (isStandalone) {
  baseConfig.pages.index.template = './pkg/opni/index.html';
}

baseConfig.productionSourceMap = false;

module.exports = baseConfig;

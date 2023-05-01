# Opni UI
This is an extension for [Rancher Manager](https://github.com/rancher/rancher) and standalone UI which allows you to administrate Opni.

View the [Opni documentation](https://opni.io/installation/opni) for more in-depth info on how to install and use Opni.

## Install as Extension
You will need to point the UI to a running instance of Rancher, here's a [quickstart guide](https://docs.ranchermanager.rancher.io/pages-for-subheaders/rancher-on-a-single-node-with-docker) for setting up Rancher in a Docker container.

> This extension requires a Rancher version of `v2.7.0` or later, you can find the latest releases [here](https://github.com/rancher/rancher/releases).

1. Navigate to the Extensions page from the side-nav and Enable the Extension support.
2. Once Extension Support has been enabled you can add this repository.
3. After this repository has been added you should see Opni is available to install.

You will now have the Opni Extension installed on your cluster.

## Developing

To run a development environment you will need a working instance of Rancher to point the frontend towards. 

### Run the Extension dev environment

1. From the root directory, install the packages.

```sh
yarn install
```

2. Run the dashboard locally.

```sh
API=https://<rancher-host> yarn dev
```

### Run the Standalone dev environment

1. From the root directory, install the packages.

```sh
yarn install
```

2. Expose the `opni-internal` service.
```sh
kubectl port-forward svc/opni-internal 8888:11080 -n opni
```

3. Run the dashboard locally.

```sh
API=https://<rancher-host> OPNI_API=localhost:8888 IS_STANDALONE=true yarn dev
```



---
title: "Installation"
linkTitle: "Installation"
weight: 2
---
 
This guide will walk you through installing Opni Monitoring and adding clusters to the system. Before proceeding, please read the section on [Terminology](../reference/terminology) to familiarize yourself with a few key terms.

## Prerequisites

### Infrastructure
- One **main** cluster where the Opni Monitoring control-plane components will be installed. The main cluster must be accessible to all downstream clusters.
  
- One or more **downstream** clusters which will be configured to send metrics to the **main** cluster

- A domain name or subdomain at which to host the Opni Gateway public API. For a demo or testing installation, you can use [sslip.io](https://sslip.io). In the rest of this guide, the domain name will be referred to as `<gateway_address>`.

- All clusters must have a default storage class available.

### Dependencies

- [helm](https://helm.sh) version **3.8** or later

- [helm-diff](https://github.com/databus23/helm-diff) plugin, which can be installed with the following command:

  ```bash
  $ helm plugin install https://github.com/databus23/helm-diff
  ```

- [helmfile](https://github.com/roboll/helmfile), which can be installed using your distribution's package manager or from the GitHub release page.


## Setting up the main cluster

First, clone the opni-monitoring repo:

  ```bash
  $ git clone https://github.com/rancher/opni-monitoring
  ```

Ensure you are running helm version 3.8 or later:
 
  ```bash
  $ helm version
  version.BuildInfo{Version:"v3.8.1", GitCommit:"5cb9af4b1b271d11d7a97a71df3ac337dd94ad37", GitTreeState:"clean", GoVersion:"go1.17.8"}
  ```

### Chart configuration

Inside the opni-monitoring repo, there are a few template yaml files in `deploy/custom` which you will need to copy and edit before continuing. These files will be used to configure authentication and Cortex long-term storage.

##### 1. Authentication

* For a demo or testing installation, follow the instructions in the [Demo Auth](../authentication/noauth) section.
* For a production installation, follow the instructions in the [OpenID Connect](../authentication/oidc) section.

##### 2. Cortex

* For a demo or testing installation, you can create an empty `deploy/custom/cortex.yaml` file to use the default configuration.

* For a production installation, edit `deploy/custom/cortex.yaml` using the template as reference and the [Cortex docs](https://cortexmetrics.io/docs/configuration/configuration-file/). 

### Chart Installation

Inside the opni-monitoring repo, change directories to `deploy/`.

1. Ensure your current kubeconfig points to the main cluster.
2. Run `helmfile apply`
3. Wait for all resources to become ready. This may take a few minutes.

### DNS Configuration

1. Identify the external IP of the `opni-monitoring` load balancer. 
2. Configure `A` records such that `<gateway_address>` and `grafana.<gateway_address>` both resolve to the IP address of the load balancer (skip this step if you are using [sslip.io](https://sslip.io) or a similar service).


### Accessing the dashboard

Once all the pods in the `opni-monitoring` namespace are running, open the web dashboard:

1. Port-forward to the `opni-monitoring-internal` service:
```bash
kubectl -n opni-monitoring port-forward svc/opni-monitoring-internal management-web:management-web
```

2. Open your browser to <a href="http://localhost:12080" target="_blank">http://localhost:12080</a>.

<br />

------

## Next Steps

This section is optional, but highly recommended. 

### Adding a cluster

First, let's add the main cluster itself - this allows us to gather metrics from the Opni Gateway and Cortex. Follow these steps to add the cluster to Opni Monitoring:

#### Create a token

Tokens are used to authenticate new clusters during the bootstrap process. To create one:

1. Navigate to the **Tokens** tab in the sidebar
2. Click **Create Token**
3. Use the default fields, and click **Create**. The new token will appear in the table.

{{% alert color="warning" title="Token Expiration" %}}
All tokens will expire after a certain amount of time. The default value is 10 minutes, but you can choose any duration you like. If your token expires, you can simply create a new one.
{{% /alert %}}

#### Add a cluster

1. Navigate to the **Clusters** tab in the sidebar
2. Click **Add Cluster**
3. In the **Capabilities** drop-down menu, select **metrics**
4. In the **Token** drop-down menu, select the token we just created. When a capability and token have been selected, an install command will appear below. 
5. Check the box that says **Install Prometheus Operator** (however, if you already have Prometheus Operator installed on a cluster you want to add, leave it unchecked).
6. Click on the install command to copy it to the clipboard
7. In a terminal, ensure your `KUBECONFIG` environment variable or `~/.kube/config` context points to the cluster you want to add (in this case, the main cluster), then paste and run the command.
8. In a few seconds, you should see a green banner informing you that the cluster has been added. If you want to add any additional clusters right now, you can repeat step 7 for each cluster using the same command you copied previously.
9. Click **Finish** to return to the cluster list.

<br />

------

### Further Reading

Read the section on [Access Control](../guides/access_control) to learn how to configure roles and role bindings. Then, head to <br /> `https://grafana.<gateway_address>` in your browser to sign in.

Check out the [Guides](../guides) section for more. Happy monitoring!
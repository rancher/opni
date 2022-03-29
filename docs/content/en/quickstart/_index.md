---
title: "Quick Start"
linkTitle: "Quick Start"
weight: 2
---
 
This guide will walk you through setting up a "demo" Opni Monitoring installation. This setup is *not* production-ready. If that is what you are looking for, check out the full [Installation](../installation) guide.

{{% alert color="info" title="Important" %}}
Before proceeding, please read the section on [Terminology](../reference/terminology) to familiarize yourself with a few key terms.
{{% /alert %}}

## Prerequisites

### Infrastructure
- One **main** cluster where the Opni Monitoring control-plane components will be installed.
  - The main cluster must be accessible to all downstream clusters from a persistent DNS name. For demo purposes, you can use [sslip.io](https://sslip.io) if you do not have a domain name available.
- One or more **downstream** clusters which will be configured to send metrics to the **main** cluster

- All clusters must have a default storage class available.

### Dependencies

- Install [helmfile](https://github.com/roboll/helmfile) using your distribution's package manager or from the GitHub release page.

- Clone the opni-monitoring repo:

  ```bash
  $ git clone https://github.com/rancher/opni-monitoring
  ```

## Setting up the main cluster

### DNS Configuration

1. Identify the DNS name of your main cluster. This will be referenced in the following sections as `<gateway_address>`. 
2. Configure `A` records such that `<gateway_address>` and `grafana.<gateway_address>` both route to the IP address of your main cluster's load balancer (skip this step if you are using [sslip.io](https://sslip.io) or a similar service).

### Chart configuration

Inside the opni-monitoring repo, change directories to `deploy/charts/opni-monitoring/custom`. There are three template files in this directory which you will need to copy and edit before continuing.

1. Copy `cortex-template.yaml` to `cortex.yaml` and leave it empty.
2. Copy `grafana-template.yaml` to `grafana.yaml` and edit it as follows, substituting `<gateway_address>`:
```yaml
grafana.ini:
  server:
    domain: "grafana.<gateway_address>"
    root_url: "https://<gateway_address>"
  auth.generic_oauth:
    client_id: "grafana"
    client_secret: "supersecret" # (this can be whatever you want)
    auth_url: "http://<gateway_address>:4000/oauth2/authorize"
    token_url: "http://<gateway_address>:4000/oauth2/token"
    api_url: "http://<gateway_address>:4000/oauth2/userinfo"
    allowed_domains: example.com # (this can be whatever you want, but remember it for later)
    role_attribute_path: grafana_role
    use_pkce: false
tls:
  - secretName: grafana-tls-keys
    hosts:
      - "grafana.<gateway_address>"
ingress:
  enabled: true
  hosts:
    - "grafana.<gateway_address>"

```

3. Copy `opni-monitoring-template.yaml` to `opni-monitoring.yaml` and edit it as follows, substituting `<gateway_address>`:

```yaml
auth:
  provider: noauth
  noauth:
    discovery:
      issuer: "http://<gateway_address>:4000/oauth2"
    clientID: grafana
    clientSecret: supersecret # (this must match client_secret in grafana.yaml)
    redirectURI: "https://grafana.<gateway_address>/login/generic_oauth"
    managementAPIEndpoint: "opni-monitoring-internal:11090"
    port: 4000
gateway:
  hostname: "<gateway_address>"
  dnsNames:
    - "<gateway_address>"
```

### Chart Installation

Inside the opni-monitoring repo, change directories to `deploy/`.

1. Ensure your current kubeconfig points to the main cluster.
2. Run `helmfile apply`
3. Wait for all resources to become ready. This may take a few minutes.


### Accessing the dashboard

Once all the pods in the `opni-monitoring` namespace are running, open the web dashboard:

1. Port-forward to the `opni-monitoring-internal` service:
```bash
kubectl -n opni-monitoring port-forward svc/opni-monitoring-internal management-web:management-web
```

2. Open your browser to <a href="http://localhost:12080" target="_blank">http://localhost:12080</a>.

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
7. In a terminal, ensure your `KUBECONFIG` environment variable or `~/.kube/config` context points to the main cluster, then paste and run the command.
8. In a few seconds, you should see a green banner informing you that the cluster has been added. Click **Finish** to return to the cluster list.


9. Repeat the above steps for any additional clusters you want to add.

#### Next Steps

Read the section on [Access Control](../guides/access_control) to learn how to configure roles and role bindings. Then, head to <br /> `https://grafana.<gateway_address>` in your browser to sign in.

{{% alert color="warning" title="Demo Authentication" %}}
With the demo setup, there is no authentication - when signing in to Grafana, you will instead be presented with a list of all known users in the system (the set of all subjects in the current list of role bindings) and can choose any user to view metrics from the perspective of that user. 
{{% /alert %}}

Check out the [Guides](../guides) section for more. Happy monitoring!
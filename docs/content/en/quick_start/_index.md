---
title: "Quick Start"
linkTitle: "Quick Start"
weight: 2
---
 
This guide will walk you through setting up a "demo" Opni Monitoring installation. This setup is *not* production-ready. If that is what you are looking for, check out the full [Installation](/installation) guide.

{{% alert color="info" title="Important" %}}
Before proceeding, please read the section on [Terminology](/architecture/terminology) to familiarize yourself with a few key terms.
{{% /alert %}}

## Prerequisites

### Infrastructure
- One **main** cluster where the Opni Monitoring control-plane components will be installed.
- One or more **downstream** clusters which will be configured to send metrics to the **main** cluster

{{% alert color="info" title="Important" %}}
The main cluster must be accessible to all downstream clusters from a persistent DNS name.
{{% /alert %}}

### Dependencies
- All clusters should have the [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) helm chart installed, with grafana and the default prometheus deployment disabled:

  ```bash
  $ helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  $ helm repo update
  $ helm install -n monitoring kube-prometheus prometheus-community/kube-prometheus-stack --set grafana.enabled=false --set prometheus.enabled=false --wait
  ```

- Install [helmfile](https://github.com/roboll/helmfile) using your distribution's package manager or from the GitHub release page.

- Clone the opni-monitoring repo:

  ```bash
  $ git clone https://github.com/rancher/opni-monitoring
  ```


## Setting up the main cluster

### DNS Setup

1. Identify the DNS name of your main cluster. This will be referenced in the following sections as `<gateway_address>`. 
2. Configure `A` records such that `<gateway_address>` and `grafana.<gateway_address>` both route to the IP address of your main cluster's load balancer.

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

3. Copy `opni-monitoring-template.yaml` to `opni-monitoring.yaml` and edit it as follows:
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
  dnsNames:
    - "<gateway_address>"
```

### Chart Installation

Inside the opni-monitoring repo, change directories to `deploy/`.

1. Ensure your current kubeconfig points to the main cluster.
2. Run `helmfile apply`
3. Wait for all resources to become healthy. This may take a few minutes.


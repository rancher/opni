---
title: "Demo Auth"
linkTitle: "Demo Auth"
description: "Configure a demo authentication provider"
weight: 10
---

The `noauth` auth provider can be used for demo purposes or for testing. 

### How it works

This demo auth mechanism allows Opni Monitoring to be its own OpenID Provider. When signing in to Grafana, instead of being redirected to an external OAuth provider, you will be redirected to a simple login page containing a list of all known users in the system (the set of all subjects in the current list of role bindings). Simply select the user you want to sign in as and you will be redirected back to Grafana, logged in as that user. All users will be a Grafana admin.

### Configuration

1. Edit `deploy/custom/grafana.yaml` as follows, substituting `<gateway_address>` for the public DNS name of your main cluster or load balancer:
```yaml
grafana.ini:
  server:
    domain: "grafana.<gateway_address>"
    root_url: "https://grafana.<gateway_address>"
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

2. Edit  `deploy/custom/opni-monitoring.yaml` as follows, substituting `<gateway_address>`:

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

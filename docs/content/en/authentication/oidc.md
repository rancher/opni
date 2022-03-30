---
title: "OpenID Connect"
linkTitle: "OpenID Connect"
description: "Using OpenID Connect with Opni Monitoring"
weight: 1
---

Opni Monitoring supports OpenID Connect for generic user authentication and authorization. Any OpenID Connect provider can be used. 

#### Configure your OpenID Provider
1. Create a new client ID and secret. 
2. Set an allowed callback URL to `https://grafana.<gateway_address>/login/generic_oauth`
3. Set an allowed logout URL to `https://grafana.<gateway_address>`

#### Edit Opni Monitoring configuration

1. Edit  `deploy/custom/opni-monitoring.yaml` as follows:

```yaml
auth:
  provider: openid
  openid:
    // discovery and wellKnownConfiguration are mutually exclusive.
    // If the OP (openid provider) has a discovery endpoint, it should be
    // configured in the discovery field, otherwise the well-known configuration
    // fields can be set manually. If set, required fields are listed below.
    discovery:
      // Relative path at which to find the openid configuration.
      // Defaults to "/.well-known/openid-configuration".
      path: "/.well-known/openid-configuration"
      // The OP's Issuer identifier. This must exactly match the issuer URL
      // obtained from the discovery endpoint, and will match the `iss' claim
      // in the ID Tokens issued by the OP.
      issuer: ""  # required
    
    // Optional manually-provided discovery information. Mutually exclusive with 
    // the discovery field (see above). If set, required fields are listed below.
    wellKnownConfiguration:
      issuer: ""                  # required
      authorization_endpoint: ""  # required
      token_endpoint: ""          # required
      userinfo_endpoint: ""       # required
      revocation_endpoint: ""
      jwks_uri: ""                # required
      scopes_supported: []
      response_types_supported: []
      response_modes_supported: []
      id_token_signing_alg_values_supported: []
      token_endpoint_auth_methods_supported: []
      claims_supported: []
      request_uri_parameter_supported: false

    // The ID Token claim that will be used to identify users ("sub", "email", etc.). 
    // The value of this field will be matched against role binding subject names.
    // Defaults to "sub".
    identifyingClaim: "sub"
```

2. Edit `deploy/custom/grafana.yaml` according to the documentation at <https://grafana.com/docs/grafana/latest/auth/generic-oauth>

```yaml
grafana.ini:
  auth.generic_oauth:
    client_id: 
    client_secret:
    auth_url:
    token_url:
    api_url:
    signout_redirect_url:
    allowed_domains:
    role_attribute_path:
    use_pkce:

```

3. Apply the configuration using `helmfile sync`.
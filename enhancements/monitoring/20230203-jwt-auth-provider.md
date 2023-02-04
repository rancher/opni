# JWT (JSON Web Token) Authentication Provider

## Summary:
This proposal describes a new auth provider that will allow authentication using static/pre-generated JWTs, and replace `noauth`.

## Use case:
This will replace the legacy `noauth` provider. The `jwt` provider will allow authenticating (to e.g. Grafana) as a specific user without needing to configure an external auth provider. It will be suitable for demo purposes, testing, and trying out Opni. Unlike `noauth` however, it will be flexible and secure enough to work in production environments.

## Benefits:
This will enable several new features to be possible, such the addition of a "system admin" user who will have access to all Opni clusters.
It will also make it much easier for new users to try out opni without needing to configure an external auth provider, and without using the old noauth provider (which was originally just for demo purposes). Furthermore, it will significantly reduce the amount of configuration required up-front when deploying Opni.

## Impact:
The entire `noauth` authentication provider will be removed. This will require updates to the gateway config/crd, and associated documentation.

## Implementation details:
This will be implemented in such a way as to support using https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/jwt/

High level details:
1. The gateway will serve a JWKS endpoint, accessible only inside the central cluster, which can be used by Grafana (and any other clients) to verify JWTs signed by the gateway.
2. The gateway will allow authenticating to its protected HTTP APIs with a JWT supplied in a well-known HTTP header.
3. The gateway will allow loading static tokens from files on disk, which can be mounted into the gateway pod as a secret. Additionally, the gateway may provide some means of generating new JWTs on demand for a specific user.
4. The RBAC-based cluster access logic does not change, as the JWT will only be used to authenticate a user, not to authorize them to access specific clusters.
5. The `MonitoringCluster` controller will be updated to configure Grafana to use JWT auth if it is enabled in the gateway config.

## Acceptance criteria:
- [ ] A user can authenticate to Grafana in a browser using a JWT token copied from a secret.
- [ ] Tokens can be marked as revoked if they are compromised. Note: It may be sufficient to rotate/delete a key in the JWKS to revoke all tokens signed with that key, or the gateway may need some extra logic to track which tokens are valid (tbd).

## Supporting documents:
https://grafana.com/docs/grafana/latest/setup-grafana/configure-security/configure-authentication/jwt/

## Dependencies:
None

## Risks and contingencies:
Opensearch doesn't appear to support an equivalent of the Grafana JWT auth provider, but this is not currently a requirement.

## Level of Effort:
Estimated time required: ~3 weeks
1 week to remove the noauth provider, and implement and test the JWKS endpoint in the gateway
1 week to implement and test controller logic for configuring Grafana to use JWT auth
1 week to implement and test gateway+controller logic for token generation and revocation


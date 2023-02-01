# Tenant Impersonation

## Summary:
Tenant impersonation is a feature that will allow metrics written by privileged agents (e.g. the local agent) to optionally be forwarded to cortex under a different tenant id.

## Table of contents
* [Motivation](#Motivation)
* [Implementation Details](#Implementation-Details)
* [Scale and performance](#Scale-and-performance)
* [Security](#Security)
* [High availability](#High-availability)
* [Testing](#Testing)

## Motivation:
In a multi-tenant environment, if a user only has access to a subset of clusters not including the central cluster, they will not be able to fetch the `opni_cluster_info` metric, as it is stored under the gateway's tenant id. Without `opni_cluster_info`, the user will not be able to see the names of the clusters they have access to inside Grafana.

## Implementation Details

The initial design to solve this problem was to have a "global tenant" that would be used to store metrics that all tenants would have access to. However, that design was determined not to be feasable to solve the problem at hand, because all tenants would have access to metrics that include descriptions of other tenants they may not have access to.

This new design would allow a privileged agent to provide metric samples with specific labels identifying the tenant to impersonate for each sample. The gateway will be able to identify such samples using well-known metadata, patch labels as necessary, then switch the tenant id when pushing the samples to the Cortex distributor.

In the case of `opni_cluster_info`, when Cortex runs its tenant federation logic to aggregate the results of queries across multiple tenants, it will group all of the `opni_cluster_info` metrics from each tenant along with their respective labels into a single logical metric when queried for. This also makes the process of vector-matching the cluster names with cluster labels much easier, as the tenant IDs will match already without needing to do complex label manipulation within promql.


## Scale and performance:
Distributing metrics across tenants will not change the overall number of metric samples that need to be stored.

There is a performance impact to intercepting metric samples in the remote write hot-path inside the gateway, which is why we will only do this under certain conditions - the metrics must be sent from the privileged local agent (identified via session attribute) and the remote-write request must have a specific header indicating the agent wishes to use tenant impersonation.

## Security:
The gateway will only allow tenant impersonation from the privileged local agent. This agent goes through an extra authentication step on each connection. It verifies that it is the local agent by using a secret key only accessible to the gateway and agent in the central cluster. It will then be assigned a well-known session attribute that can be checked by the gateway. No other agents will be able to use tenant impersonation.

## High availability:
This should not impact high availability.

## Testing:
This feature will have thorough unit testing to ensure proper security and performance characteristics.
# Title:

Opni Alerting uses Cortex AlertManager as a dispatch backend

## Summary:

Opni Alerting builds a single big routing configuration file for AlertManager based on the user's settings. Building these big files eventually becomes a performance bottleneck for the Alerting OpsServer. The server requires building the monolith file whenever the configuration changes on any cluster.

Therefore, Opni-Alerting should seek to build smaller configuration files, namely by building configuration files specific to each cluster and applying them to a multi-tenant AlertManager solution : Cortex-AlertManager

## Use case:

- Users want to scale their upstream Alerting Cluster to handle many agents

## Benefits:

- Performance & scalability optimizations & many cortex native improvements to AlertManager
- Separating clusters logically and physically -- improves fault tolerance
- Less dependency management headaches for main Opni repository
- Better integration with Metrics' Cortex cluster

## Impact:

- Cortex AlertManager uses an eventual consistency model, increasing the median time of propagating configuration updates to any Opni-Alerting Component in a reconciler loop from ~30 seconds, minus opni router build time, to approximately 1 to 2 minutes.
- Cortex AlertManager on a filesystem `storage backend` may become less than ideal when compared to Vanilla AlertManager

## Implementation details:

- Opni Alerting uses the existing embedded cortex binary to run cortex AlertManager
- A new AlertingAdmin server v2 deploys cortex AlertManager's as the Alerting Cluster unit
- AlertingAdmin server v2 `ClusterConfiguration` implements cortex AlertManager configuration
- Build Opni-routing configuration on a per cluster basis, with keys matching cluster id
- Ops Server sync stream, responsible for syncing Opni-Alerting configurations to the AlertingCluster, uses `(key, config)`, where key is a cluster id instead of a global key
- `Sidecar Syncer` server v2 that applies sync changes via the cortex API
- AlertManager adapters must be modified with an `AlertManagerApiOption` that handles `proxying` to underlying cortex AlertManager instances : for example, moving `/api/v2/status` to `/<tenant>/prom/alertmanager/api/v2/status` when enabled

## Acceptance criteria:

- [ ] AlertingAdmin v2 server
- [ ] Router configuration building on a per cluster basis
- [ ] AlertManager adapter can proxy requests to AlertManager through CortexAPI
- [ ] `Sidecar Syncer` v2 server that works with Cortex AlertManager

## Supporting documents:

## Dependencies:

- An identifiable global cluster id, for AlertConditions of the type `MonitoringBackend` or `LoggingBackend` which aren't necessarily attached to any agent ids

## Risks and contingencies:

| Risk                                                  | Contingency |
| ----------------------------------------------------- | ----------- |
| Breaking CRD changes that cause issues during upgrade | TBD         |

## Level of effort:

3 weeks

## Resources:

1 Opni upstream cluster & 2 Opni downstream cluster

# Title:

Opni Alerting uses Cortex AlertManager as a notification system

## Summary:

Opni Alerting builds a single big routing configuration file for AlertManager based on the user's settings. Building these big files eventually becomes a performance bottleneck for the Alerting OpsServer. The server requires building the monolith file whenever the configuration changes on any cluster.

Therefore, Opni-Alerting should seek to build smaller configuration files, namely by building configuration files specific to each cluster and applying them to a multi-tenant AlertManager solution : Cortex-AlertManager

## Use case:

- Users want to scale their upstream Alerting Cluster to handle many agents
- Users want to have a greater amount of optimization control over their Alerting Cluster

## Benefits:

### User Benefits

- Performance & scalability optimizations
- Separating opni clusters logically and physically in the notification system -- improves fault tolerance
  - In our current cluster if the process running AlertManager crashes then the messaging system breaks for all clusters

### Developer Benefits

- Less dependency management headaches for main Opni repository
- many cortex native improvements to AlertManager
  - Memberlist management, cluster meshing & API aggregations made easy
- Better & easier integration with Metrics' Cortex cluster

## Impact:

- Cortex AlertManager uses an eventual consistency model, increasing the median time of propagating configuration updates to any Opni-Alerting Component in a reconciler loop from ~30 seconds, minus opni router build time, to approximately 1 to 2 minutes.
- Cortex AlertManager on a filesystem `storage backend` may become less than ideal when compared to Vanilla AlertManager, so
  having an S3 configuration available is a good idea.
- Cortex AlertManager has some limitations around embedding user template, API key & cert files
  - limits to using non external files (directly in AlertManager configuration) for storing receiver information

## Implementation details:

### Compute

- Opni Alerting uses the existing embedded cortex binary to run cortex AlertManager
- A new AlertingAdmin server v2 deploys cortex AlertManager's as the Alerting Cluster unit
- Alerting Cluster deployments are controlled via an `Alerting Cluster` CRD separated from the gateway CRD

  - Gateway address
  - Cortex deployment configurations
  - Cortex CLI arguments

### Logic

- Build Opni-routing configuration on a per cluster basis, with keys matching cluster id. Currently, a global key builds the router configuration.

- Ops Server sync stream, responsible for syncing Opni-Alerting configurations to the AlertingCluster, uses `(key, config)`, where key is an opni cluster id instead of a global key
- `Sidecar Syncer` server v2 applies configuration changes via the cortex API
- AlertManager adapters must be modified with an `AlertManagerApiOption` that handles `proxying` to underlying cortex AlertManager instances : for example, moving `/api/v2/status` to `/<tenant>/prom/alertmanager/api/v2/status` when enabled

### HA Guarantees

- AlertManager adapters must maintain a member-list from Cortex when making stateful API calls to ensure it aggregates over available members
- The cluster gossip intervals must be estimated from the `Cortex runtime args`, and strictly validate `group_wait` in router construction

### UI changes

- UI must implement the API contract for the AlertingAdmin server v2
- Expose an advanced configuration with all the Cluster configuration settings for AlertingAdmin server v2

## Acceptance criteria:

- [ ] Opni Alerting deploys a CortexAlertManager cluster
- [ ] Guarantee HA state is consistent
- [ ] Alerts & messages are dispatched on a per-cluster basis
- [ ] Router configurations are built on a per-cluster basis
- [ ] AlertManager adapter can proxy requests to AlertManager through CortexAPI
- [ ] `Sidecar Syncer` server implements an adapter to Cortex AlertManager

## Supporting documents:

## Dependencies:

- An identifiable global cluster id, for AlertConditions of the type `MonitoringBackend` or `LoggingBackend` which aren't necessarily attached to any agent ids

## Risks and contingencies:

| Risk                                                  | Contingency |
| ----------------------------------------------------- | ----------- |
| Breaking CRD changes that cause issues during upgrade | TBD         |

## Level of effort:

2 weeks

## Resources:

1 Opni upstream cluster & 2 Opni downstream cluster

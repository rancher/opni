# Title:

Sync discovered Prometheus alerting rules

## Summary:

Opni-Monitoring has an existing framework for syncing downstream Prometheus rules to its Cortex Cluster. The proposal is to sync the discovered alerting rules to Opni-Alerting via an Alerting agent capability.

## Use case:

- Sync workload specific alerting rules already present in user's downstream cluster(s)

In particular targeting a common & important use case for SRE's already using AlertManager as an alerting system: deploying alerting rules in Prometheus operator via infrastructure CI scripts like terraform

## Benefits:

- Sync existing user configurations
- No pain when using alerts specific to their infrastructure / build version
- No pain when users update their workload alerts, especially those deployed via infrastructure CI
- Towards a zero configuration Opni-Alerting production setup

## Impact:

- Segregating the alert rule sync server into an Alerting downstream capability
- Relabelling incoming synced alerting rules as `OpniReadOnly="true"`, `OpniSynced="true"`

## Implementation details:

- `AlertCondition` protocol buffer messages must replace their tag system with the Prometheus matcher labels system, see https://github.com/rancher/opni/pull/972
- When Ops Server performs its syncs, updated the list of attached endpoints on `AlertCondition` with the endpoints that match its label(s) -- WUO more optimized way of doing this
- Migrate tag list on `AlertCondition` to `label matchers` list in storage client data migration, return a validation error informing the user to update the rule in their downstream
- Read only restriction should only apply to the contents of the Prometheus query, but any additional Opni-Alerting features will still be applicable
- Basic capabilityv1 alerting agent
- Alerting downstream capability that implements a `rule syncer` server specific to alerting rules

### UI

- Alerting capability on downstream clusters -- enable through UI similarly to logging & monitoring
- UI would need to make clear when listing alarms that the synced conditions are synced

## Acceptance criteria:

- [ ] Alerting downstream plugin
- [ ] Alerting rule sync server
- [ ] Migrate tags on `AlertCondition` protocol buffer message to matcher labels
- [ ] Read only restrictions on AlertCondition

## Supporting documents:

## Dependencies:

- Full endpoint integration with AlertManager : https://github.com/rancher/opni/pull/972
- Condition Status optimization : https://github.com/rancher/opni/pull/971
- Cortex AlertManager as a unit in Alerting Cluster (OEP pending):
  - Identifying upstream cluster with a `wellknown uuid`

## Risks and contingencies:

| Risk                        | Contingency                                        |
| --------------------------- | -------------------------------------------------- |
| OTEL collector rule syncing | implement an OTEL plugin for alerting rule syncing |

## Level of Effort:

3 weeks

1 week agent plugin + sync server
1 week AlertCondition labelling system, read-only system, synced system
1 week heavy e2e/integration testing

## Resources:

1 Upstream Opni Cluster & 1 Downstream Opni Cluster

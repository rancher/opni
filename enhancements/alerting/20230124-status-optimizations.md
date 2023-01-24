## Title

Alert Condition Status Optimizations

## Summary

Getting the status of a single alarm isn't expensive, however getting the status of many alarms at once can be an expensive process -- hurting UX & scalability.

## Use case

Customers scale up to many alarms & want to check their status.

## Benefits

- Optimize Opni Alerting resource usage
- Improved user experience when using the Alerting UI

## Impact

- Status will no longer be fine grained, as Alert Status will be periodically batched across various sources

## Implementation details

### Current

![](./images/alerting/condition-status-no-cache.png)

### Proposed

Backends like Cortex and AlertManager return raw lists of data when getting states -- the Alerting Plugin caches these API calls based on service descriptor / HTTP path and serves their data in memory.

In addition, calls to a particular Alarm's status are cached to avoid traversing many long lists of raw information.

The new `ListStatus` API handles listing the Condition Specs and Getting the Status in the same operation for both convenience and performance.

![](./images/alerting/condition-status-cache.png)

## Acceptance criteria

- [ ] Extend the Storage ClientSet with an in memory cache to cache calls to APIS
- [ ] Serve a read-through cache for Status
- [ ] `ListStatus()` ConditionServer API which batches ListConditions() & Status() calls.

## Supporting documents

Addresses https://github.com/rancher/opni/wiki/Alerting-Condition-Server#performance-issues

## Dependencies

- [ ] Alerting Storage ClientSet : https://github.com/rancher/opni/pull/942

## Risks and contingencies

| Risk                                                          | Contingency                                                                             |
| ------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| The cache isn't populated -- therefore calls remain expensive | Populate cache when the alerting plugin starts / once the Alerting Cluster is installed |

## Level of effort

1 week:

- day 1 to 3 : Setting up generic & reusable API call cache
- day 4 : Implementing API logic
- day 5 : Performance & scalability tests

## Resources

1 Upstream Opni Cluster

# Title: Additional Cortex Config Options

## Summary:
We propose exposing additional Cortex configuration options to enable users to customize some aspects of the managed Cortex cluster in Opni.
The options include:
- Workload replica count for distributor, ingester, querier, compactor, and store gateway
- Compactor block ranges
- Compactor intervals (cleanup and compaction)
- Some of the global resource limits that are currently disabled/hard-coded ("max X per Y")
- Querier resource/time limits (concurrency, samples, lookback/store query ranges).

## Use case:
All Opni users can benefit from additional configuration options. For example, users with many agents can scale up certain Cortex components to handle the increased load, while users with fewer agents can scale down to save on costs.

## Benefits:
- Users can customize the Cortex cluster to fit their needs.

## Impact:
This should not have any impact on existing systems, as the configuration changes will be fully backwards-compatible.

## Implementation details:
The additional config options will be added to the `cortexops.ClusterConfiguration` message, and corresponding CLI flags and UI elements will be added to control these options.

## Acceptance criteria:
- The additional options can be configured through the CLI.
- The additional options can be configured through the UI.
- Upgrading Opni does not break existing configurations.

## Supporting documents:
N/A

## Dependencies:
N/A

## Risks and contingencies:
- Incorrect configuration could break Cortex or cause data loss, so we should do our best to validate any user-configurable options.

## Level of Effort:
2 days to implement backend+CLI, 3 days for tests (1 week total), 1 day for UI

## Resources:
N/A

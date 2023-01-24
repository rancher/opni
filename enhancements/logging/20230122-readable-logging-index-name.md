# Title: Readable Logging Index Name

## Summary:

In order for users to select an agent log index in OpenSearch, they select from a list of cluster IDs. This makes it difficult to select the right index when there are multiple connected agents. This proposal aims to add human-readable index names, to simplify the process of selecting indexes.

## Use case:

When an Opni Gateway has many connected agents, it becomes cumbersome to search through a list of IDs to find the one which matches the target agent.

## Benefits:

By viewing a list of human-readable names, it will be clearer to the user which index is associated to which agent. While the current agent IDs do uniquely identify which agent each index refers to, the purpose of each cluster is clearer than an abstract alphanumeric string (ex `web-services` vs `0194fdc2-fa2f-4cc0-81d3-ff12045b73c8`).

## Impact:

## Implementation details:

The idea for this proposal is to add logic surrounding the `opni cluster rename` command. When the user changes the name of an agent cluster, 

1. If the cluster already has a name:
   1. replace the name label
   2. remove the pre-existing name label
2. Create a new alias pointing to the existing index for the agent

For this implementation to make the most sense, the names passed to `opni cluster rename <cluser> <name>` should be unique. While the implementation would still work with duplicate names, the user would see the logs from both agents in the dashboard when the user selects the index.

## Acceptance criteria:

- [ ] Running `opni cluster rename <cluster> <name>` should add a new index called `<name>` to the index dropdown list

## Supporting documents:

## Dependencies:

## Risks and contingencies:

## Level of Effort:

This addition should only take 1 week for implementation and testing.

## Resources:

The additional resource consumption will be negligible.
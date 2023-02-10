# Title: Readable Logging Name

## Summary:

Currently, when users select an agent cluster for log analytics, they select from a list of cluster IDs. This makes it difficult to select the right agent among many. This proposal aims to add human-readable agent cluster names, to help differentiate the cluster names for the user.

In the Opni UI, we will adda a new "Clusters" tab which will be responsible for displaying the available clusters and their metadata to the user.

## Use case:

When an Opni Gateway has many connected agents, it becomes cumbersome to search through a list of IDs to find the one which matches the target agent.

## Benefits:

By viewing a list of human-readable names, it will be immediately clear which cluster you are selecting. While the IDs do uniquely identify each agent, it's not immediately clear what responsibilities each agent has. Human-readable names will make agent responsibility clearer, as well as being easier to remember.

## Impact:

## Implementation details:

Cluster names will be stored in a new `cluster-metadata` index, which will store a mapping of the cluster id, cluster name, and potentially some other data as needed in the future. The UI can fetch the list of clusters and their human-readable names from this index, and perform a join in the UI to properlly display the logs.

In the logger plugin we will need to watch for cluster events so we can properlly handle when cluster events:

| Event   | Action                                                      |
|---------|-------------------------------------------------------------|
| Created | Add a new document with the name set to the cluster id      |
| Updated | Update the cluster metedata index with the new cluster name |
| Deleted | Do nothing                                                  |

## Acceptance criteria:

- [ ] When a new cluster is added, add a new entry to the cluster metadata index
- [ ] Running `opni cluster rename <cluster> <new-name>` should
  - [ ] Update the cluster metedata index with the new cluster name
  - [ ] User should be able to view the new cluster name in the cluster UI tab

## Supporting documents:

## Dependencies:

## Risks and contingencies:

## Level of Effort:

This addition should take 1.5 weeks for implementation and testing. The aim is to have this up for the March 1st release.

## Resources:

Minimal increase, since the new index is expected to be fairly small.

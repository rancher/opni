# Title: Readable Logging Name

## Summary:

Currently, when users select an agent cluster for log analytics, they select from a list of cluster IDs. This makes it difficult to select the right agent among many. This proposal aims to add human-readable agent cluster names, to help differentiate the cluster names for the user.

## Use case:

When an Opni Gateway has many connected agents, it becomes cumbersome to search through a list of IDs to find the one which matches the target agent.

## Benefits:

By viewing a list of human-readable names, it will be immediately clear which cluster you are selecting. While the IDs do uniquely identify each agent, it's not immediately clear what responsibilities each agent has. Human-readable names will make agent responsibility clearer, as well as being easier to remember.

## Impact:

## Implementation details:

Cluster names will be stored in a new `cluster-details` index, which will store a mapping of the real cluster name to the new name. The UI can fetch the list of clusters and their human-readable names from this index. When new clusters are added, a new document should be added to this index for that cluster with the name set to its id.

The logs for each cluster will also have a new `cluster` field which will specify the name of the cluster which originated the log. When the user renames a cluster, it will trigger an [Update by Query](https://opensearch.org/docs/1.1/opensearch/rest-api/document-apis/update-by-query/) for all the cluster's logs.

By leveraging the Opni [task](https://github.com/rancher/opni/tree/main/pkg/task) functionality we can track and expose the document update progress to the user. Similarly, if a user renames a cluster before the full rename is complete, we can use the OpenSearch [Tasks](https://opensearch.org/docs/1.1/opensearch/rest-api/tasks/) to cancel the existing update operation.

## Acceptance criteria:

- [ ] Running `opni cluster rename <cluster> <new-name>` should
  - [ ] Trigger an update-by-query for all documents for that cluster
  - [ ] Update the cluster name index with the new cluster name
  - [ ] User should be able to view the new cluster name in the cluster dropdown
- [ ] When a new cluster is added, add a new entry to the cluster name index
- [ ] Users should be able to query for cluster logs using the cluster name rather than id

## Supporting documents:

## Dependencies:

## Risks and contingencies:

## Level of Effort:

This addition should take 2 weeks for implementation and testing

## Resources:

Some experimentation is needed to determine how costly these update by queries will be. Otherwise, there will be a negligible increase is resource utilization.
package v1alpha

/**
- Each alert in opni-alerting is a forest of control flow actions.

- Each control flow action maps to references to two distinct trees, before and after.

- Each tree is a binary tree of AlertNodes.

- Each leaf alert node contains a reference to an alert condition that is not an alert composition action (and, or).

- Each non-leaf alert node contains a reference to an alert condition that is an alert composition action (and, or).

*/

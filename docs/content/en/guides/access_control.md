---
title: "Access Control"
linkTitle: "Access Control"
description: "Configuring role-based access to clusters"
weight: 1
---

Opni Monitoring uses Role Based Access Control (RBAC) to control which clusters a given user is allowed to see. If you are familiar with RBAC in Kubernetes, it works the same way. 

In Kubernetes, RBAC rules are used to determine which API resources a given User or Service Account is allowed to access. Similarly, in Opni Monitoring, RBAC rules are used to determine which clusters a user has access to. When a user makes an authenticated query, the system will evaluate the current RBAC rules for that user and determine which clusters to consider when running the query.

### Labels

Clusters in Opni Monitoring have an opaque ID and can have a number of key-value labels. Labels are the primary means of identifying clusters for access control purposes. Labels in Opni Monitoring are functionally similar to labels in Kubernetes, and follow these rules:
- Labels must have unique keys, and there is a one-to-one mapping between keys and values.
- Labels cannot have empty keys or values
- Label keys must match the following regular expression:

    ```
    ^[a-zA-Z0-9][a-zA-Z0-9-_./]{0,63}$
    ```

- Label values must match the following regular expression:

    ```
    ^[a-zA-Z0-9][a-zA-Z0-9-_.]{0,63}$
    ```

### Roles

A role is a named object representing a set of permissions. It contains rules that match clusters by ID or by the cluster's labels. A role can use any/all of the following matchers:

##### 1. Label Matchers

A role can match clusters by specifying an explicit `key=value` label pair that will match a single label with the exact key and value specified. No partial matching or globbing is performed.

##### 2. Label Selector Expressions

A role can also match clusters by using a Kubernetes-style label selector. These selectors are more versatile, and can match labels using a variety of rules, such as:
- Matching if a given key exists
- Matching if a given key does not exist
- Matching if the value for a given key is in a list of allowed values
- Matching if the value for a given key is not in a list of allowed values

##### 3. Explicit Cluster IDs

Clusters can also be explicitly added to a role by ID. It is not recommended to use explicit cluster IDs as a primary means of access control, but they can be useful in situations where you want to add an exception to an existing role or a temporary override. Label-based selectors are much more flexible, and should be preferred in general.

### Role Bindings

A role binding is a named object that attaches one or more users ("subjects") to a role. When evaluating RBAC rules for a given user, the system will look up all role bindings attached to that user, then use the union of the associated roles to determine which clusters the user is allowed to see.


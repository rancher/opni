---
title: "Terminology"
linkTitle: "Terminology"
description: "Definitions of commonly used terms and phrases"
weight: 1
---
 
It is important to understand the following terms and phrases before you get started with Opni Monitoring:

### Multi-Cluster

Multi-Cluster (as in "multi-cluster monitoring") refers to operations that involve multiple independent Kubernetes clusters. 

### Multi-Tenant/Multi-Tenancy

A *tenant* refers to a user or group of users who share a common access with specific privileges to the software/data instance.

In the context of Opni Monitoring, "multi-tenancy" refers to the feature that allows multiple users to each have access to a different arbitrary subset of all available data.

{{% alert color="warning" title="Important" %}}
In the Cortex documentation, the terms "tenant", "user", and "org" are used interchangeably, and all refer to what Opni Monitoring calls "clusters". 

In Cortex terminology, "multi-tenancy" refers to the ability to federate a single request across multiple \[cortex\] tenants (where "federation" refers to the aggregation of the results of several individual requests to appear as one).
{{% /alert %}} 

### (Main|Upstream|Opni Monitoring) Cluster

"Main Cluster", "Upstream Cluster", and "Opni Monitoring Cluster" all refer to the cluster where Opni Gateway, Cortex, and Grafana are installed. Metrics from all clusters are sent to the Opni Gateway in this cluster, and are processed and stored by Cortex.

### Downstream Cluster

This refers to a cluster running the Opni Agent and Prometheus Agent which sends all its metrics (using Prometheus Remote-Write) to the main cluster.

{{% alert color="info" title="Note" %}}
The Opni Agent and Prometheus Agent are often also installed into the main cluster, to scrape metrics from the Opni Gateway and Cortex. However, we would still refer to that cluster as the main cluster.
{{% /alert %}}

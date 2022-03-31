---
title: "Managing Clusters"
linkTitle: "Managing Clusters"
description: "Creating and deleting clusters, and adding capabilities to existing clusters"
weight: 2
---

### Cluster Capabilities

Each cluster in Opni Monitoring has a set of capabilities that correspond to the high-level features available to that cluster. Currently, there are two capabilities: `metrics` and `logs`. A cluster can have one or both of these capabilities. When adding a cluster, depending on the capability you select, different features will be available.

### Adding a new cluster

#### Prerequisites

1. The new cluster must be able to reach the Opni Gateway API endpoint.
2. You have previously created a token to use for the new cluster. Reusing the same token for multiple clusters is fine, as long as it has not yet expired.

#### Installation

Adding a new downstream cluster will require installing Kubernetes resources in that cluster. Different capabilities will install different resources. Use the Opni Monitoring dashboard to set up the new cluster by following the steps below:

1. Navigate to the **Clusters** tab in the sidebar
2. Click **Add Cluster**
3. In the **Capabilities** drop-down menu, select the capability you want to install. 
4. Choose an available token from the **Token** drop-down menu. When a capability and token have been selected, an install command will appear below. This command will be specific to the selected capability.
5. Depending on the selected capability, some options may appear in the UI above the install command. Changing these options will adjust the install command in some way. If any options are marked with a red asterisk, that indicates the option is required.
6. Click on the install command to copy it to the clipboard.
7. In a terminal, ensure your `KUBECONFIG` environment variable or `~/.kube/config` context points to the new cluster, then paste and run the command.
8. In a few seconds, you should see a green banner informing you that the cluster has been added. Click **Finish** to return to the cluster list.

### Adding a new capability to an existing cluster

The process of adding a new capability to an already existing cluster is similar to the process of adding a new cluster. The main difference is that there are additional requirements for the token you choose.

#### Join Tokens

When a token is used to add a cluster, the token permanently gains the `join` capability for that cluster. This can be seen in the tokens list after adding a cluster. The capability is displayed in the UI as `join:` followed by the first 8 characters of the cluster ID. A token with the `join` capability for a cluster may be used to add additional capabilities to that cluster.

If the token originally used to create the cluster expired or was deleted, you can create a new join token in the UI. To do this:

1. Navigate to the **Tokens** tab in the sidebar
2. Click **Create Token**
3. Under "Extended Capabilities", select **Join Existing Cluster**, then click **Create**. The new token will be created with the `join` capability for the cluster you selected.

### Deleting a cluster

To remove a cluster from Opni Monitoring, select the cluster or clusters you wish to delete from the clusters list and click **Delete**. Once deleted, the downstream cluster will no longer have permissions to access the Opni Gateway API and forward metrics or other data.

Currently, the following limitations apply with regards to deleting clusters:

- Deleting a cluster will not delete any of its stored metrics.
- Deleting a cluster will not uninstall any resources from the downstream Kubernetes cluster. 

#### Cleaning up a deleted Cluster

To clean up Kubernetes resources in a deleted downstream cluster, simply delete the `opni-monitoring-agent` namespace. It may take a while for all resources in the namespace to be fully deleted.

Alternatively, uninstalling the `opni-monitoring-agent` helm chart and deleting the `keyring-agent-<cluster-id>` secret will have the same effect.
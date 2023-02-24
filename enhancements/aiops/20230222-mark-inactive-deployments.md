# Title: 
Enhance User Experience when Cluster Agents are Disconnected


## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, there is no indication on whether or not the cluster agent is connected to the upstream Opni cluster. As a result, it is possible the user may be selecting deployments that are stale and are not producing any current logs. Hence, I propose that within the Opni Admin Dashboard there should be a mechanism to detect whether or not a given downstream cluster's agent is connected to the upstream Opni cluster and to display that information within the dashboard.

## Use case: 
This will denote any Kubernetes clusters which have a disconnected cluster agent as inactive such that when the user is updating the watchlist, for any clusters which do not have a cluster agent connected to the upstream cluster, those clusters will be marked as "Inactive". 

## Benefits: 
* Enhances user experience as now it is clear which clusters are actively shipping over logs.
* Improves insights provided by Deep Learning model.


## Impact: 
The Opni Admin Dashboard UI would need to be modified as would the AIOps gateway plugin. 

## Implementation details: 

Within the AIOps gateway plugin, a new endpoint will be created called cluster_check. Within that endpoint, a call will be made that will receive all of the current clusters which at some point were shipping logs over to the upstream Opni cluster. From there, a check will be made for which of these log shipping clusters currently has a cluster agent which is disconnected. The cluster_check endpoint will maintain a map with the key being the name of the cluster and the value being whether or not the cluster agent for that cluster is currently connected to the upstream Opni cluster. 

The Opni Admin UI service will call the cluster_check endpoint within the AIOps gateway plugin and when it displays the deployments per cluster, for any cluster which has a disconnected cluster agent it will make a note next to the cluster name that the cluster is currently not actively shipping logs. However, the user can still add deployments from that cluster to the watchlist.


## Acceptance criteria: 
* Opni Admin Dashboard UI should correctly display which clusters are actively shipping logs and which clusters have a disconnected cluster agent.

## Supporting documents: 
User Story:
As a user of Opni, I would like to know which of my current clusters which are shipping over logs to the upstream Opni cluster and which have a disconnected cluster agent.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Add changes to code base: 2 days
* Test changes: 2 days

## Resources: 
N/A
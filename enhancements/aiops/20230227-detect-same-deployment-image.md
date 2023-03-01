# Title: 
Detect deployments running within different clusters which use the same image

## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, they can go through the deployments of each cluster which is currently shipping over logs to the central Opni cluster and select the workloads of interest. However, if the same deployment image is running on several clusters, then the user has to go to each cluster's listing and select that deployment which can be a tedious process if the same deployment image is running on a large number of clusters. Hence, I propose a change

## Use case: 

## Benefits: 


## Impact: 


## Implementation details: 


## Acceptance criteria: 


## Supporting documents: 


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 

## Level of Effort: 

## Resources: 
N/A
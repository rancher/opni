# Title: 
Enable GPU Services upon submission of initial training job for workload logs.

## Summary: 
Currently, when a user would like to enable AI services, they will go to the Opni Admin Dashboard. First, they are required to enable Logging and once that is done, they will go to the AIOps panel and check the "Enable GPU Services button". When they hit the "Save" button, the GPU services are installed within the Kubernetes cluster. This includes the workload DRAIN service, the training controller service, the GPU Controller service and CPU Inferencing service. However, this feature can be a little more straight forward by simply detecting the existence of a GPU within the cluster and when the user decides to train a Deep Learning model, that is when these GPU services should be installed, rather than through a checkbox button.

## Use case: 
This will remove the "Enable GPU Services" check box and now will install the GPU services when the user decides to update the watchlist for the very first time with workloads.

## Benefits: 
* Improves UX of the Opni Admin Dashboard


## Impact: 
The impact of this feature would be on several services. The AIOps gateway plugin will need to be modified to be the one which enables the GPU services when the workload watchlist is enabled for the first time. Additionally, since the update of the watchlist will be launching the GPU services, logic will have to be added to AIOps gateway plugin to update the current watchlist of workloads into Opensearch and the training controller service will need to fetch these workloads from Opensearch upon startup. Lastly, the Opni Admin Dashboard UI will also be modified to no longer include the check box.

## Implementation details: 

The AIOps gateway plugin will need to be modified, specifically the /model/train endpoint. When a request is sent to that endpoint, in the [corresponding function](https://github.com/rancher/opni/blob/main/plugins/aiops/pkg/gateway/modeltraining.go#L21), a call will be made that will first enable the GPU services if this is the first request made. Thus, an initial function will be called which first enables the GPU services before calling the TrainModel function.

Within the Opni Admin Dashboard UI, the checkbox for enabling GPU services will be removed. 

## Acceptance criteria: 
* The GPU services should only be enabled upon the user updating the watchlist of workloads for the first time.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive workload insights through the GPU setup on my Kubernetes cluster.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Code implementation: <= 4 days
* Testing and debugging: <= 2 days

## Resources: 
N/A
# Title: 
Enable GPU Services upon submission of initial training job for workload logs.

## Summary: 
Currently, when a user would like to enable AI services, they will go to the Opni Admin Dashboard. First, they are required to enable Logging and once that is done, they will go to the AIOps panel and check the "Enable GPU Services button". When they hit the "Save" button, the GPU services are installed within the Kubernetes cluster. This includes the workload DRAIN service, the training controller service, the GPU Controller service and CPU Inferencing service. This UX can be avoided by simply detecting the availability of a GPU within the cluster and when the user creates or updates the workload log anomaly watchlist to train a Deep Learning model, that is when these GPU services should be installed, rather than through a checkbox button.

## Use case: 
This will remove the "Enable GPU Services" check box and now will install the GPU services when the user decides to update the watchlist for the very first time with workloads. Opni GPU services will automatically come up upon the creation or update of a workload log anomaly watchlist.

## Benefits: 
* Improves the usability of Opni AIOps log anomaly


## Impact: 
The impact of this feature would be on several services. The AIOps gateway plugin will need to be modified to be the one which enables the GPU services when the workload watchlist is enabled for the first time. Additionally, since the update of the watchlist will be launching the GPU services, logic will have to be added to AIOps gateway plugin to update the current watchlist of workloads into Opensearch and the training controller service will need to fetch these workloads from Opensearch upon startup. Lastly, the Opni Admin Dashboard UI will also be modified to no longer include the check box.

## Implementation details:

There are a couple of different scenarios that will be addressed.

Scenario 1: GPU is not available in central Opni cluster
In this case, the Opni Admin Dashboard will make a request to the gpu_info endpoint within the AIOps gateway. When, it receives the response that the GPU is not available. It will still display the deployments by cluster and namespace. However, the user will not be able to update the watchlist as that button will be greyed out. Additionally, a banner will be displayed within the Opni Admin Dashboard informing the user that they need to configure a GPU on this cluster to receive insights and it will point them to a link for reference.

Scenario 2: GPU is available in central Opni cluster

![GPU Services Modification Diagram](https://user-images.githubusercontent.com/8761010/221271577-ac8740a6-75a2-461c-9d8c-a74c10e0e257.png)

The AIOps gateway plugin will need to be modified, specifically the /model/train endpoint. When a request is sent to that endpoint, in the [corresponding function](https://github.com/rancher/opni/blob/main/plugins/aiops/pkg/gateway/modeltraining.go#L21), the following deployments will be enabled on the cluster. 

These deployments are:
* opni-svc-preprocessing
* opni-workload-drain
* opni-svc-training-controller
* opni-svc-gpu-controller
* opni-svc-inference
* opni-svc-opensearch-update

 When the deployments are launched, within the Opni Admin Dashboard UI, a button will appear that says "Disable GPU Services". This button when hovering over it will explain that if the user ever decides to remove a GPU from the cluster, then they can first click on the button which will clean up the GPU services from the cluster and then the user can detach the GPU node from the cluster. The button will specifically clean up these deployments within the namespace specified by the user to run Opni:
* opni-workload-drain
* opni-svc-training-controller
* opni-svc-gpu-controller
* opni-svc-inference 

Additionally, when installing any of these deployments, the watchlist of workloads will be saved by the model/train function to Nats jetstream kv storage under the name "model-training-parameters". 

When, the training controller service launches for the first time, it will check Nats jetstream kv storage for the most recent watchlist of workloads which it will then use to construct the Opensearch query before sending it to the GPU controller service.

As for the Opni Admin Dashboard UI, the checkbox for enabling GPU services will be removed. 

## Acceptance criteria: 
* The GPU services should only be enabled when user updates a watchlist of workloads for the first time overall or after disabling GPU services.
* The "Disable GPU Services" button should remove the opni-workload-drain, opni-svc-training-controller, opni-svc-gpu-controller and opni-svc-inference deployments from the cluster.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive workload insights through the GPU setup on my Kubernetes cluster.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Code implementation: <= 3 days
* Testing and debugging: <= 2 days
* Documentation: 2 days

## Resources: 
* EC2 instance for building plugin image 
* NVIDIA GPU 

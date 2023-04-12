# Title: 
Move storage of DRAIN and Deep Learning Models from Seaweed to Nats Object Storage.

## Summary: 
Currently, when either a DRAIN model or Deep Learning model is persisted, it is saved within Seaweed. However, one problem with Seaweed is that it is not persistent itself. So, if the pod is ever restarted, then any data saved within that instance is permanently gone. Thus,I propose that DRAIN and Deep Learning models should now be stored through Nats Object Storage instead of using Seaweed. 

## Use case: 
This will remove the dependency of the AI services on using Seaweed for persistence of models from DRAIN and the Deep Learning algorithm. As a result, if the user ever chooses to disable AI services, the previous model will still be persisted as it is part of Nats object storage.

## Benefits: 
* Improves the usability of Opni AIOps log anomaly

## Impact: 
The impact of this feature would be on several services. Both the pretrained DRAIN and workload DRAIN services would now be persisting models to Nats object storage instead of Seaweed. Additionally, the training controller, GPU controller, CPU Inferencing service and the pre-trained inferencing services for the control-plane, Rancher and Longhorn models would all need to be modified to store and fetch models from Nats object storage.

## Implementation details:

Currently, the Python Nats client does not support object storage. However, there is a pending pull request within the Nats repository to include object storage within the Python client. However, for the time being, it will be necessary to include the build of the Nats CLI within the Docker images for the DRAIN, GPU Controller, Training Controller and Inferencing services.

This would be accomplished by the steps 
```
zypper refresh
```

and 
```
zypper install wget
```

Then for the GPU controller service, when it is time to persist a newly trained Deep Learning model, it will use the os.system call to make a request to the Nats CLI to add the newly trained model to the Nats object storage. First it would need to create the bucket 
```
nats object add opnilogmodelbucket
```




## Acceptance criteria: 
* The GPU services should only be enabled when user updates a watchlist of workloads for the first time overall or after disabling GPU services.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive workload insights through the GPU setup on my Kubernetes cluster.


## Dependencies: 
Besides the requirement of having Nats running with Jetstream on the cluster, no other requirements are needed.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Code implementation: <= 2 days
* Testing and debugging: <= 2 days

## Resources: 
* EC2 instance for building plugin image
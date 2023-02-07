# Title: 
Improve estimate for workload model training time

## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, once they hit the button to update the watchlist, a default estimate of 1 hour is provided for the training of a Deep Learning model. I propose an approach where based on the number of logs that are to be fetched initially, we estimate the 

## Use case: 
This will give a more accurate estimate of the completion time of the Deep Learning model trained on the watchlist of workloads. 

## Benefits: 
* Provides clearer initial time estimates of training time of Deep Learning model than simply saying 1 hour always.


## Impact: 
This would impact the AIOps Gateway plugin, Training controller service and Admin UI backend. The AIOps Gateway plugin would no longer by default be updating the model training status to have 1 hour remaining of training time every time it receives a request. Instead the remaining time would likely be a negative number. The UI service will need to process the negative number accordingly. The training controller service would be modified so it sends an initial estimate to the /model/statistics endpoint based on the number of log messages and the average time to complete a training job.

## Implementation details: 

 Within the /model/train endpoint within the AIOps plugin, when it is time to train a new Deep Learning model, set the remaining time for training a new model to -1 instead of 3600 which is the previous value passed in for 1 hour. The UI service will also need to be updated to parse the -1 value to indicate that the estimate is currently being calculated. 

 Once the AIOps gateway sends the workloads watchlist through Nats, the training controller service will now count the total number of log messages it will be fetching from Opensearch and then it will be estimate the time to train in the following manner.

 Let B represent the time it takes to process one batch size of 32 training logs.
 Let C represent the total number of logs that are to be fetched within Opensearch.

 The estimated time will be calculated through this approach

 (C / 32) * B

 That result will be sent to the /model/statistics endpoint.

## Acceptance criteria: 
* When it is time to train a new model, initially there should be no estimate provided for when model will be trained and initial estimate will be based on the number of logs present within the last hour and the average amount of time it takes to process 100 batches of log messages.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive an accurate initial estimate on how long it will take for AIOps insights to be ready for workload logs.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Come up with estimated amount of time that it takes to process through 1 batch of size 32 of training logs: less than a day
* Update gateway plugin and training controller service: 1 day
* Update UI service: 1 day
* Test changes: 1 day

## Resources: 
N/A
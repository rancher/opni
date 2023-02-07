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

 Within the /model/train endpoint within the AIOps gateway plugin, when it is time to train a new Deep Learning model, the stage will be set to fetch and then the workload watchlist is sent to the training controller service through Nats.

 First, get the number of logs L that are to be fetched within Opensearch. 
 
 Then that number should be divided by 10000 as that is the number of Opensearch scrolling operations that are necessary to fetch all those logs. This gives us S.

 S = L // 10000
 
 Then with an estimate of 5 seconds per scrolling operation, compute the estimated time it would take for all logs to be fetched within Opensearch. This will be denoted as TFF which stands for Time for Fetching.
 
 TFF = S * 5

 Next, when it comes to training the Deep Learning model, take the maximum between L, the number of logs that are
 to be fetched within Opensearch and 64000 which is the current maximum number of samples of training data for the Deep Learning model. This will be denoted as M. 
 
 M = max(L, 64000)
 
 Then, take M and multiply it by 0.9. This is because 90% of that dataset will be used for training the Deep Learning model and the remaining 10% will be used for validation of the model. This will be denoted as MTD for Model Training Data. Round TM down to ensure that it is an integer.
 
 MTD = round(0.9 * M)
 
 Then, using 0.45 seconds as the amount of time for the model to train on one batch of size 32 in one epoch, multiply that rate by the number of batches calculated. This will provide the estimated time for training one epoch of the model. This will be denoted as ET for Epoch Time.
 
 ET = MTD * 0.45
 
 As the Deep Learning model trains for 3 epochs, multiply this by 3 to get the estimated training time of the model. This will be denoted as MTT for Model Training Time.
 
 MTT = 3 * ET
 
 Now, to compute the ETA for when Opni log anomaly insights will be ready, simply add TFF which is the estimated time to fetch all logs and MTT the estimated time to train the Deep Learning model and this gives us ETA.
 
 ETA = TFF + MTT
 
 ETA will be computed in seconds and the result will be written to the /model/statistics endpoint.

## Acceptance criteria: 
* When it is time to train a new model, initially there should be no estimate provided for when model will be trained and initial estimate will be based on the the estimated time for fetching the logs and for training the model.
* When data is being fetched, the Admin Dashboard will show a banner which says "The deployment watchlist is being updated. Data is being fetched. AI Insights should be ready in [Estimated Time]".
* When the model is being trained, the Admin Dashboard will show a banner which says "The deployment watchlist is being updated. It is X% complete and estimated to be done in [Estimated Time]"

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive an accurate initial estimate on how long it will take for AIOps insights to be ready for workload logs.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Update gateway plugin and training controller service: 1 day
* Update UI service: 1 day
* Test changes: 1 day

## Resources: 
N/A
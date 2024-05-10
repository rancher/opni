# Title: 
Improve estimate for workload model training time

## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, once they hit the button to update the watchlist, a default estimate of 1 hour is provided for the training of a Deep Learning model. I propose an approach where our estimate for when the AIOps insights will be ready will be broken down into three components. The first component estimates how long it will take to fetch all logs from Opensearch. The second component estimates how long it takes to mask those log messages. The third component estimates the time taken to train the Deep Learning model. 

## Use case: 
This will give a more accurate estimate of the completion time of the Deep Learning model trained on the watchlist of workloads. 

## Benefits: 
* Provides clearer initial time estimates of training time of Deep Learning model than simply saying 1 hour always.


## Impact: 
This would impact the AIOps Gateway plugin, Training controller service and Admin UI backend. The AIOps Gateway plugin would no longer by default be updating the model training status to have 1 hour remaining of training time every time it receives a request. Instead the remaining time would likely be a negative number. The UI service will need to process the negative number accordingly. The training controller service would be modified so it sends an initial estimate to the /model/statistics endpoint based on the number of log messages and the average time to complete a training job.

## Implementation details: 

 Within the /model/train endpoint within the AIOps gateway plugin, when it is time to train a new Deep Learning model, the stage will be set to fetch and then the workload watchlist is sent to the training controller service through Nats.

**This is the process to compute the initial ETA.**

First, get the number of logs L that are to be fetched within Opensearch. 
 
Then that number should be divided by 10000 as that is the number of Opensearch scrolling operations that are necessary to fetch all those logs. This gives us S.
```math
 S= L // 10000
 ```
 
 Then with an estimate of 5 seconds per scrolling operation, compute the estimated time it would take for all logs to be fetched within Opensearch. This will be denoted as TFF which stands for Time for Fetching.
 ```math
 TFF = S * 5
 ```

 Next, when it comes to estimating the time it takes to mask log messages, with an estimate of 0.005 seconds to mask one message, compute the estimated amount of time to mask all log messages. This will be denoted as TTM for Time to Mask.
```math
 TTM = L * 0.005
 ```

 Next, when it comes to training the Deep Learning model, take the maximum between L, the number of logs that are
 to be fetched within Opensearch and 64000 which is the current maximum number of samples of training data for the Deep Learning model. This will be denoted as M. 
 ```math
 M = max(L, 64000)
 ```
 
 Then, take M and multiply it by 0.9. This is because 90% of that dataset will be used for training the Deep Learning model and the remaining 10% will be used for validation of the model. This will be denoted as MTD for Model Training Data. Round TM down to ensure that it is an integer.
 ```math
 MTD = round(0.9 * M)
 ```
 
 Then, using 0.45 seconds as the amount of time for the model to train on one batch of size 32 in one epoch, multiply that rate by the number of batches calculated. This will provide the estimated time for training one epoch of the model. This will be denoted as ET for Epoch Time.
 ```math
 ET = MTD * 0.45
 ```
 
 As the Deep Learning model trains for 3 epochs, multiply this by 3 to get the estimated training time of the model. This will be denoted as MTT for Model Training Time.
 ```math
 MTT = 3 * ET
 ```
 
 Now, to compute the ETA for when Opni log anomaly insights will be ready, simply add TFF which is the estimated time to fetch all logs, TTM which is the estimated time to mask all log messages and MTT the estimated time to train the Deep Learning model and this gives us ETA.
 ```math
 ETA = TFF + TTM + MTT
 ```
 
 ETA will be computed in seconds and the result will be written to the /model/statistics endpoint.

**Compute ETA while one of the components is ongoing**
As each component is reached during the training phase, the ETA calculated will be updated based on the actual time taken so far.

Hence, when it comes to actually fetching data from Opensearch, using S which is denoted as the number of Opensearch scrolling operations. Compute the ETA for fetching the data by taking the time it took to dump logs, denoted as DT so far and divide it by the current scrolling operation I by the total number of scrolling operations. Let ES denote this term.
```math
ES = DT / (I / S)
```
At this point
```math
ETA = ES + TTM + MMT
```

When the masking is being done, the estimated time for computing when the masking will be finished denoted as EM can be computed as follows with MC the Cth current log message that is being masked, L is the total number of log messages that were fetched from Opensearch and TM the total time that has been spent on the masking operation so far:
```math
EM = TM / (C / L)
```

With TF being the total actual time it took to fetch all logs from Opensearch, the ETA would now be calculated as:
```math
ETA = TF + EM + MMT
```

Lastly, for the model training component, that part has already been implemented within the GPU controller but the same approach as before will be used for computing the ETA.

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
# Title: 
Fetch last hours worth of logs for each workload specified in the watchlist as part of training data for Deep Learning Model

## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, the corresponding logs for all of the workloads specified are fetched from Opensearch within the last hour. However, by having an absolute start and end time to fetch log messages, there is a possibility that some of the workloads specified by the user will not have any log messages present during that time. Hence, a new approach is proposed where for each workload specified by the user in the watchlist, all of the logs within the last 24 hours will be fetched.

## Use case: 
This will filter out workload log messages with anomalous keywords from the training data of the Deep Learning model. 

## Benefits: 
* Improves insights provided by Deep Learning model.


## Impact: 
There would not be any major impact to the current system. The training controller now would send a list of individual queries to run and the GPU controller service will now run a list of several queries to fetch the logs for training the Deep Learning model.

## Implementation details: 

First, within the training controller service, a query will be made to fetch the last occurring timestamp of the log for each of the specified workloads. From that point onwards, take the maximum number of logs possible and divide that number by the number of workloads specified. This will indicate the maximum number of logs to fetch for each workload.

```math
LogsPerWorkload = MaxTotalLogs // NumWorkloads
```

Next, fetch the latest timestamp for each workload specified. Once the timestamps are retrieved, then for each workload take the timestamp fetched and subtract 86400 (number of seconds in 24 hours) to get the starting time range. Create a separate query dictionary for each workload and store them in a JSON object.

After all workload queries have been stored, send over the result to the GPU controller service through Nats.

The GPU controller service will now fetch the training logs by running each Opensearch query and storing the logs within a list.

Afterwards, the same procedure as current is followed which involves processing each of the log messages through masking, creating the vocabulary and training the model.

## Acceptance criteria: 
* Deep Learning models should now be trained on the last hours worth of logs for each workload specified.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive the most accurate log anomaly insights from the workloads I added to the watchlist.

Additionally, when it comes to fetching the latest timestamp for a log message, this query can be used as a [reference point](https://github.com/rancher/opni-training-controller/blob/main/training_controller/prepare_training_logs.py#L237).

## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort: 
* Code implementation: 2 days
* Testing and debugging: <= 2 days

## Resources: 
N/A
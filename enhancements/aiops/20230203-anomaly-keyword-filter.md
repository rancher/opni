# Title: 
Filter out anomalous keywords from training dataset of workload logs for Deep Learning model

## Summary: 
Currently, when a user would like to train a Deep Learning model on a watchlist of workloads, the corresponding logs for all of the workloads specified are fetched from Opensearch within the last hour. While a check is made within the Opensearch query to omit any logs which were previously marked as anomalous, there is no check made for any workload logs which contain keywords that are typically associated with anomalous logs. I propose that we maintain a list of anomalous keywords and for any log message that is fetched from Opensearch, we do not include it in the training dataset if it contains at least one word from the list of anomalous keywords.

## Use case: 
This will filter out workload log messages with anomalous keywords from the training data of the Deep Learning model. 

## Benefits: 
* Acts as safe guard to avoid adding clearly anomalous log messages to training dataset.
* Improves insights provided by Deep Learning model.


## Impact: 
There would not be any major impact to the current system. The query made to Opensearch to fetch the filtered log messages will need to be updated but no architecture change is necessary.

## Implementation details: 
Error keywords:
- fail
- error
- fatal
- exception
- timeout
- unavailable
- OOM
- crash
- connection refused
- network error
- deadlock
- out of disk
- high load

The GPU controller service, which handles both model training and inferencing, will query OpenSearch to retrieve workload logs from the past hour that have not been marked as anomalous and do not contain any of the designated anomalous keywords. The retrieval process will be performed using scrolling, where each query to OpenSearch will retrieve 10,000 logs at a time. This is done in preparation for training a Deep Learning model on a designated watchlist of workload logs.

After retrieving all log messages for the training dataset, the GPU controller will proceed to create the required model vocabulary and commence training the model, using the same steps as previously described.


## Acceptance criteria: 
* Deep Learning models should only be trained on workload logs within the last hour that are not marked as anomalous and do not contain any of the designated anomalous keywords.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive the most accurate log anomaly insights from the workloads I added to the watchlist.

This is the query to be used. It has been verified in the dev console in a long running opensearch cluster:
```
GET logs*/_search
{
  "query": {
    "bool": {
      "must_not": [
        {
          "query_string": {
            "default_field": "log",
            "query": "fail OR error OR fatal OR exception OR timeout OR unavailable OR OOM OR crash OR connection refused OR network error OR deadlock OR out of disk OR high load"
          }
        }
      ]
    }
  }
}
```

## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
| Risk                                                                                                     | Contingency                                                                                    |
|----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| Normal log messages that happen to contain anomalous keywords are filtered out of the training dataset.  | We may need to add additional guidelines to filter out log messages from the training dataset. |

## Level of Effort: 
* Add changes to code base and test changes: 1 day

## Resources: 
N/A

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
We will create and maintain a list of keywords that are associated with anomalous logs. For now, we will go with the keywords: "error", "fail", "fatal" and "exception".

 Once, it is time to train a Deep Learning model on a designated watchlist of workload logs, within the GPU controller service which is responsible for both model training and model inferencing, it will send a query to Opensearch to retrieve all workload logs within the last hour that have not been marked as anomalous and do not have any of the anomalous keywords within its text. The retrieval of workload logs is done in a scrolling manner where each call to Opensearch fetches 10000 logs at a given time.

 Once all of the log messages have been retrieved for the training dataset, the GPU controller will follow the same steps as before to create the necessary model vocabulary and then begin training the model.

## Acceptance criteria: 
* Deep Learning models should only be trained on workload logs within the last hour that are not marked as anomalous and do not contain any of the designated anomalous keywords.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive the most accurate log anomaly insights from the workloads I added to the watchlist.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
| Risk                                                                                                     | Contingency                                                                                    |
|----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| Normal log messages that happen to contain anomalous keywords are filtered out of the training dataset.  | We may need to add additional guidelines to filter out log messages from the training dataset. |

## Level of Effort: 
* Come up with list of anomalous keywords: less than a day
* Design query to filter out logs with anomalous keywords: less than a delay
* Add changes to code base: 1 day
* Test changes: 1 day

## Resources: 
N/A
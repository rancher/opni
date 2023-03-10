# Title: 
Allow user to specify error keywords for Opni log anomaly insights

## Summary: 
Currently, when it is time to train a Deep Learning model upon the user editing the watchlist of workloads, there is a list of anomalous keywords which are used to filter out any workload log messages that contain any of these keywords. This list of keywords is something that the user currently doesn't have any control over as it is predetermined. I propose that when the user is updating the watchlist of workloads, that they also have the option to update which error keywords they would like to be used for filtering and tracking. 

## Use case: 
This will allow the user to add additional error keywords to the list of immutable error keywords which will be used to filter out any log messages that are fetched for training the Deep Learning model. 

## Benefits: 
* Gives user the ability to add additional anomalous keywords to enhance user experience.
* Improves insights provided by Deep Learning model.


## Impact: 
The Opni admin dashboard would have to be modified to show the initial keywords. Additionally, the training controller service will also need to be modified in order to now update its list of anomalous keywords and not just use the default listed words.

## Implementation details:
Currently, this is the list of error keywords that are to be used by default.
- fail
- error
- fatal
- exception
- timeout
- unavailable
- crash
- connection refused
- network error
- deadlock
- out of disk
- high load

This is an immutable list so the user cannot remove any of these keywords. Within the Opni admin dashboard, there will be a box which shows the error keywords that are used by default. The user will then have the option to remove any keywords that they would not like to use as well as include any additional keywords. The user will also make sure to select the workloads of interest to the watchlist. There will be an update button which the user will then hit which sends the keywords to the AIOps gateway endpoint /train/model which will send the workloads to train on and the error keywords to filter out. The /train/model endpoint will then send that data over to the training controller service.

The training controller service will then receive workloads that are part of the watchlist as well as the user specified anomalous keywords through Nats. It will then use the specified list of anomalous keywords to create the Opensearch query which will then be sent to the GPU controller service. After this, the same approach for fetching the logs and training the Deep Learning model will be followed.

## Acceptance criteria: 
* Deep Learning models should only be trained on workload logs within the last hour that are not marked as anomalous and do not contain any of the designated anomalous keywords.

## Supporting documents: 
User Story:
As a user of Opni, I would like to receive the most accurate log anomaly insights from the workloads I added to the watchlist by specifying my own custom keywords.


## Dependencies: 
Besides the requirement of having Opni AIOps already enabled with an NVIDIA GPU setup on a cluster, no additional dependencies are present.

## Risks and contingencies: 
N/A

## Level of Effort:
* Modify the Opni admin dashboard UI to take in anomalous keywords: 2 days  
* Add changes to code base and test changes: 1 day

## Resources: 
N/A

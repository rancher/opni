# Title: 
Identify prometheus metrics that match known anomaly chart patterns

## Summary: 
This enhancement proposal offers a new feature to match prometheus metrics to a set of known pre-defined anomaly chart patterns. This feature will make operators' life easier in the process of outage diagnosis.

## Use case: 
* Monitoring metrics in a multi-cluster setup is challenging. Operators could be responsible for hundreds of clusters and each cluster has tons of different metrics. When an outage occurs, it's difficult to identify root-cause metrics to assist outage diagnosis. Users need tools to help them reduce the scope and identify anomaly metrics faster.

## Benefits: 
* makes operators' life easier in the process of outage diagnosis.
* improve user experience. Metric data will get condensed and categorized into insights that are much easier to digest.
* reduce mean time to resolve outages. This method will shorten the time operators taken to find out root cause.
* reduce economic loss.

## Impact: 
* No direct impact to existing system in terms of performance. However, if this new feature is deployed in a `proactively` in the future, then it might require some resources as it would have to continuously compute for all metrics.

## Implementation details: 
* pattern classification process. Inspired by this paper, https://netman.aiops.org/wp-content/uploads/2021/10/wch_ISSRE-1.pdf. 
	a. A simple metric anomaly detection algorithm, that predicts every metric either normal or anomaly. An example method is Kolmogorov-Smirnov test.
	b. A machine-learning or deep-learning model will be pre-trained for pattern classification. This would require some abnormal metric data collection and to apply data augmentation techniques. Once model is trained, it will be applied to anomaly metrics from step a and categorize them into a few pre-defined patterns.
	c. analyse report. A report should be generated to summarize the notable patterns.
* metric data collection. Data will be pulled from Cortex as chunks. Likely it would require data from last a few hours before an outage.
* back-end apis for frontend to connect to.
* UI. 
	a. User would need to manually trigger this feature, so there needs to buttons attached to each alerts to trigger this analysis.
	b. the analyse report should be visualized in UI.

## Acceptance criteria: 
* pattern classification accuracy should be good. Threshold to be decided. 
* analyse report generated correctly.
* pulling metrics data should be efficient.
* UI. Buttons to trigger this new feature work as expected and the analyse report gets visualized.

## Supporting documents: 
https://netman.aiops.org/wp-content/uploads/2021/10/wch_ISSRE-1.pdf. 

## Dependencies: 
* the Opni metric back-end enabled.

## Risks and contingencies: 
* Risks: None
* Contingencies: None. 

## Level of effort: 
A rough estimation:
* week 1-3:
    * experiments and implementation for the pattern classification model. This includes the data collection phase and iterations of model training & evaluation.
    * pull data from Cortex to test models
    * implement the back-end apis
    * implement UI.
* week 4: Integration and testing.

## Resources: 
* A upstream Opni cluster and several downstream cluster.
* Likely need a GPU to train the classification model(convolutional neural network). Model inference probably won't require GPU.

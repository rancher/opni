# Title: 
Identify prometheus metrics that match known anomaly chart patterns

## Summary: 
This enhancement proposal offers a new feature to match prometheus metrics to 2 types of known pre-defined anomaly chart patterns. For each time of the patterns, a grafana dashboard that visualize all anomaly metrics that match to the patterns will be dynamically generated for user to consume. This feature will make operators' life easier in the process of outage diagnosis.

The 2 types of pre-defined patterns:
<img width="1401" alt="Screen Shot 2023-02-22 at 1 54 28 PM" src="https://user-images.githubusercontent.com/4568163/220768433-82056fd1-494c-4afe-9822-77b0566fd99d.png">

## Use case: 
* Keeping track of all metrics in a multi-cluster setup is challenging. Subject matter experts could be responsible for hundreds of clusters and each cluster has tons of different metrics. Often times, SMEs are required to identify metrics that are important to a workload. Because only certain metrics are selected, there can be blind spots - as other metrics may capture details of the system overlooked by the metrics curated by SMEs. When an outage occurs, it's difficult to identify root-cause metrics to assist outage diagnosis. Users need tools to help them reduce the scope and identify anomaly metrics faster.

## Benefits: 
* Makes operators' life easier in the process of outage diagnosis.
* Improve user experience. Metric data will get condensed and categorized into insights that are much easier to digest.
* Reduce mean time to resolve outages. This method will shorten the time operators taken to find out root cause.
* Reduce economic loss.

## Impact: 
* No direct impact to existing system in terms of performance. However, initially this feature will only be deployed in `reactive mode` -- it's only triggered when an alert is fired. If this new feature is deployed in a `proactive mode`(which means it runs periodically) in the future, then it might require more resources as it would have to continuously compute for all metrics.

## Implementation details: 

* Pattern classification process. 
	a. A simple metric anomaly detection algorithm, that predicts every metric either normal or anomaly. An example method is Kolmogorov-Smirnov test.
	b. A machine-learning or deep-learning model will be pre-trained for pattern classification. This would require some real abnormal metric data with human labels, and then apply data augmentation techniques to them. Once model is trained, it will be applied to anomaly metrics from step a and categorize them into a few pre-defined patterns.
	c. Analyse report. A report should be generated to summarize the notable patterns.
* Metric data collection. Data will be pulled from Cortex as chunks. Likely it would require data from last a few hours before an outage.
* Back-end apis for frontend to connect to.
* UI. 
	a. User would need to manually trigger this feature, so there needs to buttons attached to each alerts to trigger this analysis.
	b. The analyse report should be visualized in UI.

## Acceptance criteria: 
* Pattern classification accuracy should be good. Threshold to be decided. 
* Analyse report generated correctly.
* Pulling metrics data should be efficient.
* UI. Buttons to trigger this new feature work as expected and the analyse report gets visualized.

## Supporting documents: 
https://netman.aiops.org/wp-content/uploads/2021/10/wch_ISSRE-1.pdf. 

## Dependencies: 
* the Opni monitoring back-end enabled.

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

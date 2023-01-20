# Title: 
Anomaly Pattern Matcher of Metric Data

## Summary: 
	Monitering metrics in a multi-cluster setup is challenging. Operators could be assigned hundreds of clusters to monitor and each cluster has tons of different metrics. When an outage occurs, it's difficult to identify root-cause metrics to assist outage diagnosis. This OEP aims to make such investigation easier by proposes an approach to categorize metrics into a few pre-defined patterns, such that during an outage disgnosis, operators can prioritize investigating metrics with those patterns.

## Use case: 
	* Reactive - Only triggered for outage diagnosis. Run this method during the diagnosis to discover anomalous metric patterns.

## Benefits: 
	a. improved user experience. Metric data will get condensed and categorized into insights that are much easier to digest.
	b. reduce mean time to resolve outages. This method will shorten the time operators taken to find out root cause.
	c. reduce economic loss. This is derived from b.

## Impact: 
	* No direct impact to existing system in terms of performance. However, if this new feature is executed `proactively` in the future, then it might require some resources as it would have to continuously compute for all metrics.

## Implementation details: 
	* pattern classification process. Inspired by this paper, https://netman.aiops.org/wp-content/uploads/2021/10/wch_ISSRE-1.pdf. 
		a. A simple metric anomaly detection algorithm, that will be applied to every metric `m` to predict if `m` is either normal or anomaly. An example is Kolmogorov-Smirnov test.
		b. A machine-learning or deep-learning model will be pre-trained for pattern classification. This would require some abnormal metric data collection and potentially apply data augmentation techniques. Once model is trained, it will be applied to anomaly metrics from step a and categorize them into a few pre-defined patterns.
		c. analyse report. A report should be generated to summarize the notable patterns.
	* metric data collection. Data will be pulled from Cortex as chunks. Likely it would require data from last a few hours before an outage.
	* backend APIs for frontend to connect to.
	* UI. 
		a. User would need to manually trigger this feature, so there needs to buttons attached to each alerts in order to trigger this analysis.
		b. the analyse report should be visualized in UI.

## Acceptance criteria: 
	* pattern classification accuracy should be very good. Threshold to be decided. 
	* analyse report generated properly.
	* pulling metrics data should be efficient.
	* UI. Buttons to trigger this new feature work as expcted and the analyse report gets visualized.

## Supporting documents: 
<head></head>ttps://netman.aiops.org/wp-content/uploads/2021/10/wch_ISSRE-1.pdf. 

## Dependencies: 
	* Opni metric backend enabled.

## Risks and contingencies: 
	* the quality of the pattern classification model. This requires experiments to validate it.

## Level of Effort: 
	A rough estimation:
	* week 1: Focus on the experiments for the pattern classification model. This includes the data collection phase and iterations of model training & evaluation.
	* week 2: implement the [pattern classification process](#Implementation details). Start to pull data from Cortex to test.
	* week 3: implement the backend APIs, put every pieces together. Start to implement the UI.
	* week 4: Integration and testing.

## Resources: 
	* A upstream Opni cluster and several downstream cluster.
	* Likely need a GPU to train the classification model(CNN). Inferencing probably won't require GPU.

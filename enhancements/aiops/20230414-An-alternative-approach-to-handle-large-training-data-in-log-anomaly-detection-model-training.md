# Title: 
An alternative approach to handle large training data in log anomaly detection model training

## Summary: 
This proposal proposes an alternative approach to resolve issues when training-data-size of log anomaly detection model is large. Compare to the approach proposed in the [streaming-dataloader OEP](https://github.com/rancher/opni/blob/main/enhancements/aiops/20230203-introduce-streaming-dataloader-in-log-anomaly-detection-model-training.md), this approach can obtain more stable model quality and suffer less from Opensearch index-scroll issue. It also makes it easier to build the vocab file.

## Use case: 
When Opni's user launches a workload log anomaly detection training job with huge size of training data (for example, 500GB logs), the current approach that loads all data into memory won't be feasible. This approach can resolve this problem because it only processes a reasonable subset of training dataset each time.

## Benefits: 
* Compare to streaming-dataloader:
	* more stable model quality than using streaming-dataloader.
	* only need to process data once when training with more than 1 epochs.
* No longer loads the whole training dataset into memory, this resolves the memory pressure issue in the scenario that training data size is large.
* Reduce data download/preprocessing time by parallelization.


## Impact: 
* This feature will prevent the model training service from running out of memory.

## Implementation details: 

Let the total size of training data be `s`(for example `s` could be 10 million)
the target size of preprocessed/masked/resampled training data be `t` (for example `t` could be `128000`)

* Opni-inference-service:
	*  Data downloading & preprocessing: 
		1. For every batch `b` of training data downloaded(say size(`b`) = 100k), pre-process and mask logs in `b` and then use [weighted random sampling](https://github.com/rancher/opni-inference-service/blob/main/opni_inference_service/opnilog_trainer.py#L120) to down-sample `b` to `sampled_b`. `size(sampled_b) = size(b) * t / s`. 
		   Return: Append `sampled_b` in each batch and return.
		2. Apply `weighted random sampling` again to re-sample the output from step 1. 
		3. Use the data from step 2 to train model.
		Apply multiple workers in step 1 can speedup the process.



## Acceptance criteria: 
* Model can be trained correctly from a large training dataset(more than 10M logs).
* The memory usage during the model training phase is predictable. Each worker will use approximately 200 MB memory. As a comparison, the existing approach uses ~ 2.5 GB memory when training data size is 2.5 million logs.
* Model training doesn't take longer than before


## Supporting documents: 

## Dependencies: 

## Risks and contingencies: 
* Risks: None
* Contingencies: None. 

## Level of effort: 
* Code implementation: <= 1 days. 
* Testing and debugging: 4 days. Requires manually test it with larger training dataset in a long running cluster many times.

## Resources: 
* A upstream Opni cluster with a GPU node attached, and 1+ downstream cluster.

# Title: 
implement streaming data-loader for log anomaly detection model training

## Summary: 
The AIOps log anomaly detection model can train a model using a streaming data-loader, such that it doesn't have to load all training dataset into memory. Instead, it will only dump a subset of training dataset each time and train with that subset of data. This feature significantly reduces the memory resource pressure in the model training service.

## Use case: 
When Opni's user launches a workload log anomaly detection training job with huge size of training data (for example, 500GB logs), the current approach that loads all data into memory won't be feasible. The streaming data-loader can resolve this problem because it only loads a reasonable subset of training dataset each time.

## Benefits: 
* Resolve the memory pressure issue in the scenario that training data size is large.

## Impact: 
* This feature will prevent the model training service from running out of memory.

## Implementation details: 
The [method that queries training data](https://github.com/rancher/opni-inference-service/blob/main/opnilog-inference-service/opnilog_trainer.py#L46) from Opensearch will return a generator, so it's iterable and it won't keep all the data in memory. It yields each scroll of the search results.
* To implement a streaming data-loader similar to [`IterableDataset`](https://pytorch.org/docs/stable/data.html#torch.utils.data.IterableDataset) in this [script](https://github.com/rancher/opni-inference-service/blob/main/models/opnilog/opnilog_parser.py). More specifically, an `__iter__()` function will be implemented in this data-loader, which returns an iterator of training dataset. Multi-processing will be applied to speed up the phase of log masking and weighted random sampling, the number of worker will be the same as the number of CPU cores assigned.
* The training method [`train()`](https://github.com/rancher/opni-inference-service/blob/main/models/opnilog/opnilog_parser.py#L75) will be slightly adjusted to the new streaming data-loader.

## Acceptance criteria: 
* Model can be trained correctly from a large training dataset.
* The memory usage during the model training phase should be within a reasonable range.


## Supporting documents: 
https://pytorch.org/docs/stable/data.html#torch.utils.data.IterableDataset

## Dependencies: 

## Risks and contingencies: 
* Risks: None
* Contingencies: None. 

## Level of effort: 
* Code implementation: <= 2 days. 
* Testing and debugging: 8 days. This includes testing the features in multiple rounds in a long running cluster, and then to fix any edge cases found in this process.

## Resources: 
* A upstream Opni cluster with a GPU node attached, and 1+ downstream cluster.

# Title: 
implement streaming data-loader for log anomaly detection model training

## Summary: 
The AIOps log anomaly detection model can train a model using a streaming data-loader, such that it doesn't have to load all training dataset into memory. Instead, it will only dump a subset of training dataset each time and train with that subset of data. This feature significantly reduces the memory resource pressure in the model training service.

## Use case: 
When Opni's user launches a workload log anomaly detection training job with huge size of training data (for example, 500GB logs), the current approach that loads all data into memory won't be feasible. The streaming data-loader can resolve this problem because it only loads a reasonable subset of training dataset each time.

## Benefits: 
* No longer loads the whole training dataset into memory, this resolves the memory pressure issue in the scenario that training data size is large.
* Reduce data download/preprocessing time by parallelization.

## Impact: 
* This feature will prevent the model training service from running out of memory.

## Implementation details: 
![Untitled Diagram drawio-9](https://user-images.githubusercontent.com/4568163/220510227-2337714b-666c-45a7-a586-eba31f28a93d.png)


* Training controller: Forms a payload with a few queries that can be used to download logs of `user-selected deployments` from Opensearch. The default number of queries is fixed value `3`, because the default number of CPUs assigned to the GPU service is 4. The GPU service needs to maintain the main process for model-training and it can spawn 3 subprocess for the streaming-dataloader.
* Function downloading data from Opensearch. The input is a query `q` from training controller. The function will download data from Opensearch with this query `q` and will return a generator. It yields each scroll(10000 logs) of the search results.
* Data-preprocessing function: 
    - input: a query `q` from training controller, this function then calls the function downloading data from Opensearch with this query `q` and iterables over each scroll of logs.
    - step1: Masking. Apply [Opni's pre-defined masking rules](https://github.com/rancher/opni-inference-service/blob/main/opni_inference_service/models/opnilog/masker.py) to logs.
    - step2: Weighted random sampling. Logs typically contain lots of duplicates, so it is sufficient to train a model with a proper sampled subset of training dataset. At this moment, the amount of training data size after this sampling method is a fixed value `192000`, and each scroll (10000) of logs will be sampled accordingly. For example, assume there will be 3840000 logs downloaded from Opensearch in total, then each scroll (10000) of logs will be sampled from 10000 to 500 logs.
    - output: Yields each masked sampled log.
* Streaming data-loader and IterableDataset:
    - IterableDataset. A custom implementation of IterableDataset similar to [`IterableDataset`](https://pytorch.org/docs/stable/data.html#torch.utils.data.IterableDataset). An `__iter__()` function will be implemented, it takes the `data-preprocessing function` as input, defines how each worker should provide an iterator to the streaming dataloader. For instance, if there are 3 queries in the payload from training-controller, then there will 3 workers spawned, each takes 1 query as input to its own copy of `data-preprocessing function`.
    - Streaming data-loader: takes the custom IterableDataset as input, serves as the input of model-training. Number of workers is equal to the number of queries from the payload from training-controller.
* Tokenize and padding batches. 
    - Tokenization. nothing new but just the [tokenization method](https://github.com/rancher/opni-inference-service/blob/main/opni_inference_service/models/opnilog/opnilog_tokenizer.py). For each log as input, tokenize the log to a lost of tokens, map each token to an integer(its `index`), return the list of token-indices.
    - Padding. For each list of token-indices, add padding token-index to make the number of token-index in the list to a fixed value, the default value of it is `64`. This list will then be converted to a pytorch tensor and returned.



## Acceptance criteria: 
* Model can be trained correctly from a large training dataset.
* The memory usage during the model training phase is predictable. Each worker will use approximately 150-200 MB memory. As a a comparison, the existing approach uses ~ 2.5 GB memory when training data size is 2.5 million logs.
* Model training doesnt take longer than before


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

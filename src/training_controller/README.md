## Training Controller Service

## Run on k8s cluster
Pre-requisites:
* Logs must be shipped from client cluster to the target (where you'll deploy preprocessing) cluster
* Have fastapi-fluentd-sink service running
* Have elasticsearch cluster installed
* Have Minio set up on cluster
* Have Nats installed on cluster
* Must have at least one GPU node (preferably K80 GPU or higher) and at least two CPUs as part of the cluster with at least 10 GiB memory as well.
* Make sure appropriate rbac is set up. 

```
* To set up rbac: kubectl apply -f rbac.yaml
* To deploy training-controller service: kubectl apply -f training_controller.yaml
```
---
---
#### Methodology
* Training controller service is subscribed to the Nats subject called "train"
* When it receives any content from this subject, it will launch the necessary steps.
* Controller will first fetch the logs from Elasticsearch that will be used by the two model training jobs: FastText with PCA and NuLog.
* Then, both FastText and NuLog models will be trained.
* Once, jobs are completed, both jobs will send a message to the Nats subject indicating that the inference service can update its models that it is currently using.
* To test this service, once you have deployed this service onto your Kubernetes cluster, you can run the sample-service job that will publish the train subject with
payload in this format
```
    payload = [{"start_ts": 1616097870000000000, "end_ts": 1619457870000000000}]
    encoded_payload_json = json.dumps(payload).encode()
```
payload is a list of dictionaries where each dictionary should have a "start_ts" and "end_ts" key with a timestamp in nanoseconds.
* You can then view the pods and jobs of your cluster to verify that the Nulog and FastText models are both undergoing training.
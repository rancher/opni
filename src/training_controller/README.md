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
* To set up rbac:, first run kubectl apply -f rbac_service_account.yaml
* Then run kubectl apply -f rbac_role.yaml
* Then, finally run kubectl apply -f rbac_role_binding.yaml
* To deploy training-controller service: kubectl apply -f training_controller.yaml
```
---
---
#### Methodology
* Training controller service is subscribed to the Nats subject called "train"
* When it receives any content from this subject, it will launch the necessary steps.
* Controller will first fetch the logs from Elasticsearch that will be used by the NuLog training job.
* Nulog model is then trained through a job.
* Once, Nulog model training has been completed, it will send a message to the Nats subject indicating that a new model is ready to be used.
*
Payload sent to the "train" Nats subject should be in this format
```
    payload = {"model_to_train": "nulog","time_intervals": [{"start_ts": 1617039360000000000, "end_ts": 1617039450000000000}, {"start_ts": 1617039510000000000, "end_ts": 1617039660000000000}]}

```
* You can then view the pods and jobs of your cluster to verify that the Nulog model is undergoing training.
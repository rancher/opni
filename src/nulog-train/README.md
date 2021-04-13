## Nulog Training Job

* This job will train a Nulog model when the training controller service receives a signal from NATS.
  * Pre-requisite: An NVIDIA GPU driver must be installed as it is required for training.
    ```
    kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.6.0/nvidia-device-plugin.yml
    The training controller service should also be running beforehand as well as should NATS, Traefik and Elasticsearch.
    ```
* The job will train a Nulog model based on logs retrieved from Elasticsearch.
*To test this service, you can send payload to the NATS subject "train" in this format
  ```
  payload = {"model_to_train": "nulog","time_intervals": [{"start_ts": TIMESTAMP_NANO, "end_ts": TIMESTAMP_NANO}, {"start_ts": TIMESTAMP_NANO, "end_ts": TIMESTAMP_NANO}, ...]}
  ```
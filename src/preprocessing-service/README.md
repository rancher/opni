## Preprocessing Service

* This service preprocesses log data from the NATS subject `raw_logs`
  * Reminder: payload-receiver-service writes to `raw_logs`
* Preprocessed logs are indexed to Elasticsearch and sent to the NATS subject `preprocessed_logs`

```
kubectl apply -f preprocessing.yaml
```
## Payload Receiver Service

* This service exposes an endpoint that logs can be sent to
  * Pre-requisite: **traefik** is already installed
* The service does some time window processing and passes the
payload to NATS.


```
kubectl apply -f payload-receiver-service.yaml
```
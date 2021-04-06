## Payload Receiver Service

* This service exposes an endpoint that logs can be sent to
  * It expects **traefik** is already installed
* The service does some time window processing and passes the
payload to the NATS.


```
kubectl apply -f payload-receiver-service.yaml
```
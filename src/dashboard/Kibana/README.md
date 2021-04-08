## deploy opendistro-es with kibana

```
1. git clone https://github.com/opendistro-for-elasticsearch/opendistro-build
2. cd opendistro-build/helm/opendistro-es/
3. helm package .
4. helm install opendistro-es opendistro-es-1.13.1.tgz
```

## setup Kibana dashboard

port forward kibana to localhost:
```
kubectl port-forward svc/opendistro-es-kibana-svc 5601:443
```
then pls visit -> `localhost:5601`  with username: `admin`  and pwd: `admin`

Import the example dashboard by click `Stack Management -> Saved Objects -> Import` and select `kibana-dashboard.ndjson` from this folder. Click Done and Check it out in Dashboard!

## setup Kibana alert and Slack notification


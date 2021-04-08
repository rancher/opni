## deploy grafana with helm
1.
```
helm repo add grafana https://grafana.github.io/helm-charts
helm install grafana grafana/grafana
```

2.
get password:
```
kubectl get secret --namespace default grafana -o jsonpath="{.data.admin-password}" | base64 --decode ; echo
```

3. portforward grafana so you can visit it locally
```
kubectl port-forward --namespace default service/grafana 3000:80
```
Navigate to http://localhost:3000, login with username: `admin` and password from step 2.


4. a way to expose grafana externally:
```
kubectl patch svc grafana -p '{"spec": {"ports": [{"port": 3000,"targetPort": 3000,"name": "https"},{"port": 3000,"targetPort": 3000,"name": "http"}],"type": "LoadBalancer"}}'
```

## setup grafana visualization dashboard
1.
on the left side of the grafana webpage, select `configuration` -> `Data Sources`, then click `Add data source` and choose `Elasticsearch`.
In HTTP section, input `http://elasticsearch-elasticsearch-master:9200` as URL
In Elasticsearch details section, index name is `<YOUR-INDEX-NAME>`, Time field name : `<YOUR-TIMEFIELD>`, for version choose `7.0+`,
In Logs section, Message field name is `<YOUR-LOG-FIELD>`

click `Save&Test`,  you should see something like `Index OK. Time field name OK.`

2.
go back to the home page and navigate to the `+` sign and select `import`, click on `UPLOAD from JSON file` and choose the `k3s-db.json` attached in this repo. Click Import to confirm and the dashboard will be there for you!

## deploy grafana with kubectl

1. create the deployment for grafana
```
kubectl create deployment grafana --image=docker.io/grafana/grafana:latest
```

2. check the deployment
```
kubectl get deployments
```

3. expose grafana to a public IP
```
kubectl expose deployment grafana --type=LoadBalancer --port=3000 --target-port=3000 --protocol=TCP
```

4. get the external IP by:
```
kubectl get services
```

you should see something like this:
```
NAME                            TYPE           CLUSTER-IP       EXTERNAL-IP                                                              PORT(S)             AGE
elasticsearch-master            ClusterIP      10.100.228.19    <none>                                                                   9200/TCP,9300/TCP   45h
grafana                         LoadBalancer   10.100.235.60    a79d65ccd6f4d44e2850d41c06bbd0c4-38040541.us-east-2.elb.amazonaws.com    3000:31445/TCP      13s
```

in this case, you can visit grafana at `a79d65ccd6f4d44e2850d41c06bbd0c4-38040541.us-east-2.elb.amazonaws.com:3000` ,
and the internal URL of elasticsearch is `http://10.100.228.19:9200`
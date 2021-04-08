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

Import the example dashboard by click `Stack Management -> Saved Objects -> Import` and select `kibana-dashboard.ndjson` from this folder. Click Done and Check out the example Dashboard!

## setup Kibana alert and Slack notification

### configure slack as alert destination

1. from your browser, browse to: https://companyname.slack.com/apps
2. Click the `Get Essential Apps` button
3. Search for and select `Incoming WebHooks`
4. Click `Add to Slack`
5. Select the channel to post alerts to
6. Hit `Add incoming Webhooks integration`
7. Copy the `Webhook URL`

8. browse to kibana at `localhost:5601`
9. Click the menu button on the left of `Home` at top left
10. Select `Alerting` and then click `Destinations`
11. Hit `Add destination`, put a name for the destination, and paste the Webhook URL from step 7 to the box `Webhook URL`, then Click `Create`

### configure monitor that sends alerts

1. Click `Alerting` and then select `Monitors`
2. put a name for `Monitor name`
3. in `Define the monitor` section, select `logs` for index and `time` for time field
4. For the query that pops up from step 3, select `For the last 1 minute(s)` and `WHERE anomaly_level is Anomaly`, click `Create`
5. put a trigger name in the `Trigger name` box, for Trigger condition, select `IS ABOVE 10`
6. scroll down to the `Configure actions` section
7. put `slack notification` as Action name, select the configured Slack Destination as Destination.
8. Put the message you want in `Message Subject` and Click `Create`

You are ready to go!

## Launch ML model training in Kibana!





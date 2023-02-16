# Title:

Alerting Integration with OpenSearch

## Summary:

Allow end-users to specify OpenSearch queries as an alarm type in Opni-Alerting

## Use case:

- End-users gain access to Opni-Alerting features on logging observability data and by extension, Opni-Alerting features on AiOps logging insights

## Benefits:

- Use Opni-Alerting with Logging & AiOps

## Impact:

- Logging will have to implement logging admin APIS for interacting with:
  - OpenSearch monitors (CRUD)
  - OpenSearch monitor status
  - OpenSearch destinations (CRUD)
- Alerting Gateway plugin will embed a logging admin client

## Implementation details:

### Logging Admin Changes

```proto
service LoggingAdminV2 {
  //....

  // ======= monitors =======

  // core.Reference refers to the monitor id

  rpc GetMonitor(core.Reference) returns (OpensearchMonitor){}
  rpc MonitorStatus(core.Reference) returns(MonitorStatus) {}
  rpc ListMonitors(google.protobuf.Empty) returns (OpensearchMonitorList){}
  rpc LoadMonitor(OpensearchMonitor) returns (core.Reference){}
  rpc DeleteMonitor(core.Reference) returns (google.protobuf.Empty){}

  rpc SilenceMonitor(core.Reference) returns (google.protobuf.Empty){}

  // destinations

  rpc ListDestinations(google.protobuf.Empty) returns (OpensearchDestinationList){}
  rpc LoadDestination(OpenSearchDestination) returns (google.protobuf.Empty){}

}

enum OpensearchAlertStatus{
  Completed = 0;
  Active = 1;
  Acknowledged = 2;
  Deleted = 3;
  Error = 4;
}

message MonitorStatus {
  repeated items = 1;
}

message OpensearchMonitor{
  core.Reference clusterId = 1;
  // same convention as metrics : doesn't make sense to wrap complex JSON data in complex grpc protobufs
  byte monitor = 2;
}

message OpensearchMonitorList{
  repeated OpensearchMonitor items = 1;
}

message OpensearchDestination{
    // same convention as metrics : doesn't make sense to wrap complex JSON data in complex grpc protobufs
  byte monitorDestination = 1;
}

message OpensearchDestinationList{
  repeated OpensearchDestination items = 1;
}
```

`MonitorStatus` wraps the [persisted alerts status](https://opensearch.org/docs/2.4/observing-your-data/alerting/monitors/#work-with-alerts) for the associated monitor.

Alerting uses `LoadMonitor` to create the OpenSearch dependencies required to activate the alarm.

`LoadMonitor` bundles the [Create monitor](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#create-a-query-level-monitor) & [Update monitor](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#update-monitor) apis depending on what operation is appropriate. The reason is to be semantically and ideologically in line with the existing internal opni alarms and cortex alarms :

1. they are stateless
2. alerting's only concern is whether or not the monitor spec is loaded

`SilenceMonitor` wraps the [acknowledge alert api](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#acknowledge-alert)

`DeleteMonitor` wraps the [Delete monitor api](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#update-monitor).

`ListDestinations` wraps the [Get destinations api](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#get-destinations)

`LoadDestination` wraps the [create destination api](https://opensearch.org/docs/2.4/observing-your-data/alerting/api/#create-destination)

### OpenSearch Query Alarm

```proto
message AlertConditionOpensearchQuery{
  core.Reference clusterId = 1;
  string queryDSL = 2;
  string trigger = 3;
  google.protobuf.Duration interval = 4;
}
```

- `clusterId` field modifies the source index on the monitor to apply only to the specified cluster

- `queryDSL` field sets the query on the monitor. query for the monitor abides by the [query DSL](https://opensearch.org/docs/latest/observing-your-data/alerting/monitors/#create-a-monitor)

- `trigger` field maps to the monitor's trigger field, it is fundamentally a `Painless` script that returns true or false

- `interval` field maps to query-level monitors' field:

```
"schedule": {
    "period": {
      "interval": 1,
      "unit": "MINUTES"
    }
  }
```

The actions field on the monitor will always be implicitly set by Opni-Alerting based on the alarm configuration.

The actions field always points to the opni-gateway address and the new trigger alerting `TriggerWithHook` api.

The logic for creating & translating these fields will go into `pkg/alerting/drivers/opensearch`.

<hr/>

For this to opensearch alarm to work we also need to accept open search alerts via an API, to forward them to alertmanager:

```proto
service TriggerAlerts {
  // ...

  rpc TriggerWithHook(google.protobuf.Any) returns (google.protobuf.Empty) {}
}
```

the `TriggerWithHook` api parses the `opensearch webhook destination` message JSON contents inside the API, and posts the information to the AlertManager cluster.

### Alerting Gateway plugin miscellaneous changes

- Embed the `LoggingAdminV2` client in the alerting gateway plugin in the existing `func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface)`

- `AlertConditionStatus` & `ListStatusAlertCondition` APIs should return `Invalidated` state for Opensearch alarms if the associated monitor doesn't exist in Opensearch. The `Reason` field on the returned state can include more details.

- `AlertConditionStatus` & `ListStatusAlertCondition` APIs need to also get the state from the Monitors using `MonitorStatus`, with the following mappings `OpenSearch -> OpniAlerting`:

  - `Acknowledged` ----> `Silence` (when an OpenSearch admin acknowledges it from OpenSearch, propagate that change to OpniAlerting in the form of a silence)
  - `Completed` ----> `Ok`
  - `Active` ----> `Firing`
  - `Error`/`Deleted` ----> `Invalidated` (when something is deleted through a source other than opni alerting, or something is severely wrong). The `Reason` field should specify additional details

- The alerting push stream responsible for pushing updates to dependencies also needs to act on some opensearch data:
  - it needs to ensure that the appropriate `webhook opensearch destination` always exists and all opensearch alarms point to that destination id
  - read silence metadata on opensearch alarms and apply `SilenceMonitor` until the `end` timestamp in the silence metadata

### UI/UX

- UI implements the OpenSearch alarm type

## Acceptance criteria:

### Logging

- [ ] Logging admin APIs that CRUD opensearch monitors
- [ ] Logging admin APIs that check Monitor status through associated persisted alerts
- [ ] Logging admin APIs for loading & listing opensearch destinations

### Alerting

- [ ] Opensearch query alarm type
  - [ ] input construction of protocol buffer to Opensearch monitors
  - [ ] extensive input validation to constructions
  - [ ] trigger hook api that accepts webhook message contents
- [ ] Opensearch monitor status mapping to OpniAlerting alarm status
- [ ] Push/apply updates to opensearch dependencies inside the alerting push stream syncing logic

## Supporting documents:

- https://opensearch.org/docs/2.4/observing-your-data/alerting/api/

## Dependencies:

- N/A

## Risks and contingencies:

| Risk                                                                                    | Contingency                                                                                                                                                                                       |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Query & trigger DSL may be too complicated to construct & validate on the alerting side | Recreate the fields of the visual editor for opensearch monitors, which would reduce the scope of allowed configurations for the query & trigger DSLs but would be easier to construct & validate |

## Level of Effort:

~ 4-5 weeks

- 5 days logging admin apis
- 5-10 days Opensearch query alarm : dsl inputs and parsing google.protobuf.Any is tricky
- 5 days misc gateway changes , e.g. non-alerting push-stream criteria to apply to external sources
- 5 days : some e2e test automation for logging + alerting integration is required due to the complexity of interactions

## Resources:

1 Opni Upstream Cluster + 1 Opni Downstream Cluster

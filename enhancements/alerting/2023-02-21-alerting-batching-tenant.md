# Title

Alerting CRUD batching configurations, aimed at multi-tenancy

## Summary

Currently, users create alerting configurations one-by-one in the UI

The alerting plugin needs to offer ways to batch apply configurations and setting default templates for specific tenants/clusters.

## Use Case

- End-users want to manually import a large amount of configurations for Opni-Alarms
- End-users want ways to batch connect conditions to endpoints
- End-users want to set defaults for all their clusters or particular clusters

## Benefits

- Improved UX
- Better management of Opni-Configurations

## Impact

- Versioning a new alerting admin API

## Acceptance criteria

- [ ] V2 Alerting admin API

  - [ ] Spec definition
  - [ ] OpenAPI generation
  - [ ] Alerting storage `clientset` migration
  - [ ] Feature parity with V1

- [ ] Batch import raw configurations (Prometheus Rule Groups) to Opni-Alerting

- [ ] Support batch routing using arbitrary AlertManager label `matchers`

- [ ] Persist Tenant Configurations (`TC`) that specify batches of alarms
  - [ ] Support a global `TC`
  - [ ] Enable scoping of tenant configurations to specific label `matchers` or cluster ids.
  - [ ] When clusters matching the labels are created, updated, or deleted or when the tenant configuration changes, the management watcher applies/deletes these configurations

**Important** `TC` will only initially support `PrometheusQuery` AlertConditions

- [ ] Alerting CLI for all alerting APIs for a more seamless batch update experience

### UI/UX

- [ ] Use v2 APIs

- [ ] When listing `Endpoint/Alarm` in table format, show routing labels
- [ ] Show severity when listing `Alarm`s in table format

- [ ] Import Prometheus rule groups to opni alerting

- [ ] Admin page for CRUDing alerting tenant configurations

## Implementation details

### V2 alerting API

- Require a new versioning of the API `alertingv2`
- Fix OpenAPI spec generations and always generate OpenAPI spec for each service

```proto
service AlarmConfig{
  // Only CRUD Alarms
  rpc CreateAlarm(...)returns(...){}
  rpc GetAlarm(...)returns(...){}
  rpc ListAlarms(...)returns(...){}
  rpc UpdateAlarm(...)returns(...){}
  rpc DeleteAlarm(...)returns(...){}
  rpc CloneAlarm(...)returns(...){}
}

service EndpointConfig{
  // Only CRUD endpoints
  rpc CreateEndpoint(...)returns(...){}
  rpc GetEndpoint(...)returns(...){}
  rpc ListEndpoint(...)returns(...){}
  rpc UpdateEndpoint(...)returns(...){}
  rpc DeleteEndpoint(...)returns(...){}
  rpc ToggleNotifications(...) returns (...) {}
}

// see below
service TenantConfig{
  rpc CreateTenantConfig(...)returns(...){}
  rpc GetTenantConfig(...)returns(...){}
  rpc ListTenantConfig(...)returns(...){}
  rpc UpdateTenantConfig(...)returns(...){}
  rpc DeleteTenantConfig(...)returns(...){}
}

// metadata aggregator for routing
service RoutingConfig{
  rpc ListRoutingRelationships(...) returns(...){}
  //...
  // Eventually can look at explictly setting tenant routing rules here
  // for plain notifications, enforce other rules we come up with,
  // or tweak the inner configuration of opni routing
}

service NotificationService{
  rpc TestAlertEndpoint(...) returns(...){}
  rpc SendAlert(...) returns(...){}
  rpc ResolveAlert(...) returns(...){}
  rpc SendNotification(...) returns(...){}

  rpc GetAlarmStatus(...) returns(...) {}
  rpc ListAlarmsWithStatus(...) returns(...) {}
  rpc ActivateSilence(...) returns(...)  {}
  rpc DeactivateSilence(...) returns (...) {}
  rpc Timeline(...) returns(...) {}
}


service RuntimeService{
  rpc ConnectRemoteSyncer(...) returns (stream ...) {}
}
```

- Aim to physically separate in alerting gateway plugin encapsulation logic for
  - config CRUD across services to a `MetadataServer`
  - runtime states & dependency management to a `ManagementServer`

```go
type MetadataServer struct{
  alertingv2.UnsafeAlarmConfig
  alertingv2.UnsafeEndpointConfig
  alertingv2.UnsafeRoutingConfig
  alertingv2.UnsafeTenantConfig
}
```

```go
type ManagementServer struct {
  alertingv2.UnsafeNotificationService
  alertingv1.UnsafeRuntimeService
}
```

#### Alerting storage `clientset` migration enhancement

A new version of the API indicates the storage `clientset` needs to migrate buckets from
one object type to another, for example : `alertingv1.AlertCondition` -> `alertingv2.Alarm`

- Have a separate buckets for each relevant alertingv2 type objects
- The alerting storage client set interfaces should be changed to use `proto.Message`
- Get and write APIs need to use safe-reflection
- Invoking the existing `clientset` API `Use` uses reflection to check contents of each bucket version, then migrate old buckets to new buckets

#### Management server details

- Embeds the existing management hook handler
- An adaptable visitor abstraction that iterates over stored alarms (without loading them all into memory, and handles different degrees of asynchronous behaviour):

  - which should handle activating/deactivating them
  - propagating metadata updates to external resources (for example, cortex rules and open search monitors)
  - triggering metadata updates directly on alarm `config` resources (required for dynamic templating message contents)

- Storage `clientset` for alerting APIs should have a `Yield` API for asynchronously fetching the next unprocessed resource from a list of resource keys

### `Protobuf` changes

- move `AlertCondition` messages to `Alarm` messages in v2 APIs

```proto
// v2
message Alarm {
  string id = 1;
  string name = 2;
  string description = 3;
  core.Reference clusterId = 4;
  // OpniSeverity is required by the Opni system
  OpniSeverity severity = 5;
  // hold implementation details
  //
  // - rate limiting configs
  // - send-resolved=yes/no
  // - last-updated
  // - silence-info
  map<string,string> properties = 6;

  // Holds message content variables from Alertmanager format
  //
  // Body is set via "OpniHeader" key
  // Description is set via "OpniSummary" key
  //
  // Users should be able to add their own custom annotation variables from the UI as well
  map<string,string> annotations = 7;

  // labels for custom routing
  map<string,string> routingLabels = 8;

  // Introduction of the new template system will deprecate this field
  AlertTypeDetails alertType = 9;
  repeated alertingv2.AttachedEndpoint attachedEndpoints = 10;
}


message AttachedEndpoint {
  string id = 1;
  // what labels this endpoint was attached from
  //
  // If none, this was manually attached by end-user
  map<string,string> source = 2;
}
```

- move `AlertEndpoint` messages to `Endpoint` messages in v2 APIs

```proto
// v2
message Endpoint {
  string id = 1;
  string name = 2;
  string description = 3;
  // Holds implementation details
  //
  // - holds receive-notification = on/off
  // - holds last updated time
  // - default/fallback rate limiting config when using label matchers
  map<string,string> properties = 4;
  // custom routing labels
  repeated prometheus.LabelMatcher routingLabels = 5;
  oneof endpoint {
    SlackEndpoint slack = 6;
    EmailEndpoint email = 7;
    PagerDutyEndpoint pagerDuty = 8;
    WebhookEndpoint webhook = 9;
  }
}
```

### Batch import configurations

```proto
message AlarmList {
    repeated Alarm items = 1;
}

service Alarms {
    //!! Does not create these rules, only converts them to the opni format
    rpc ConvertPrometheusRuleGroup(RawPrometheusRuleGroup) returns (AlarmList){}
}

message RawPrometheusRuleGroup{
  bytes data = 1;
}
```

### CRUD BatchTenantConfiguration API(s)

```proto
// Also referred to as BTC
message BatchTenantConfiguration {
    // opaque
    string id = 1;
    // name the user sets
    string name = 2;
    // manually selected clusters to apply this template to
    core.ReferenceList clusters = 3;
    // apply this template to any clusters matching these labels
    map<string,string> clusterLabelMatchers = 4;
    // flags properties of this tenant
    //
    // - use to flag global tenant configuration
    map<string,string> tenantProperties = 5;
    // alarms to apply by default to this
    AlertConditionList conditions = 6;
}

message BatchTenantConfigurationList{
    repeated items BatchTenantConfiguration = 1;
}


service TenantConfigs {
    // Tenant template
    rpc ListTenantConfig(google.protobuf.Empty) returns (BatchTenantConfigurationList){}

    rpc CreateTenantConfig(BatchTenantConfiguration) returns (google.protobuf.Empty){}

    rpc GetTenantConfig(core.Reference) returns (BatchTenantConfiguration){}

    rpc UpdateTenantConfig(BatchTenantConfiguration) returns google.protobuf.Empty{}

    rpc DeleteTenantConfig(BatchTenantConfiguration) returns google.protobuf.Empty{}
}
```

- Labeling alarms created with Tenant Configurations with the opni_tenant : <tenant_id> property is necessary so that updates can be propagated to matching alarms when tenant configurations are created, updated, or deleted.
- Add existing agent disconnect and capability unhealthy defaults to the GlobalTenant

### UI/UX

#### Import Prometheus rules

- Button in Alarm page called `Import Prometheus`
- API returns a list that should be previewed
- Users select what clusters to apply these to
- UI will have to individually create the Alarms when the user confirms

#### Tenant configurations

- Tab in the install page where you can CRUD tenant configurations
- Pasting a string invokes `ConvertPrometheusRuleGroup`

## Supporting documents

- Generic Prometheus Rule group format : https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/
- Prometheus label `matchers` `proto` : https://github.com/prometheus/prometheus/blob/main/prompb/types.proto
- AlertManager label `matchers` format : https://github.com/prometheus/alertmanager/blob/fd0929ba9fc58737a9c91f24771862692fa72d17/pkg/labels/matcher.go

## Risks and contingencies:

N/A

## Level of effort:

### `Backend`

Approximately 5 to 6 weeks

- 1 1/2 weeks v2 API

- 1 week support simple custom routing

- 1 day documentation update

- 3 days batch Prometheus rule group (requires v2 -- It is necessary to import annotations and labels.)

- 1 1/2 weeks CRUD Tenant configurations (requires v2 -- significant internal rework of v2 API will impact implementation details + other reasons)

  - 1 week API implementation
  - 2 to 3 days testing

- 1 week CLI implementation

### UI

??? days

## Resources:

- git alerting staging branch
- 1 Opni Upstream & 1 Opni Downstream cluster

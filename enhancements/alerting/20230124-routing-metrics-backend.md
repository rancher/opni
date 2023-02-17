# Title:

Alert Routing Integration with Metrics

## Summary:

Opni Alerting should allow for importing existing AlertManager configurations and make them work out of the box with both Cortex & Opni Alerting.

## Use case:

- Users have existing production-ready Prometheus AlertManager configurations they wish to use
- Production & enterprise settings on endpoints, like TLS

## Benefits:

- Users retain existing production-grade functionality of user configurations
- Less configuration for experiences users
- Use Opni-Alerting features with existing configurations
- Less overall user input to get started with Opni & Opni-Alerting
- More notification targets for Opni Alerting to send alerts to

## Impact:

Won't impact existing Opni functionality, only allows for more targets.

## Implementation details:

- Implement protocol buffer messages for each AlertManager receiver currently missing (see below)
- Extensive validation for those protocol buffers
- Building these protocol buffer messages into OpniRouter
- Sync User configurations API

**Clarification**

- Synced configurations are written to an object store through the storage client, then when the Alerting ops reconciler runs, they're built into the opni router, the opni router builds the configuration, then that configuration is pushed via the sync stream to each Alerting Cluster Unit
- File imports are written & discovered directly on the AlertManager `storage backend`
  - If cortex AlertManager, to the `cortex storage backend`
  - If vanilla AlertManager, to the Alerting cluster PVC

```proto
// As long as the Alerting Cluster backend is powered by vanilla AlertManager
// the clusterKey is set to "global", otherwise if the Alerting Cluster is
// powered by Cortex AlertManager
service AlertingAdminServer {
  //...
  rpc SyncConfig(UserConfigRequest) returns (google.protobuf.Empty){}

  rpc ListSyncConfigs(ListSyncedConfigRequest) returns (syncConfigList){}

  rpc ImportFile(UserFileImportRequest) returns (google.protobuf.Empty) {}

  rpc ListImportedFiles(ListImportedFiles) returns (google.protobuf.Empty) {}
  //....
}

message SyncConfig{
  string clusterKey = 1;
  byte config = 2;
}

message SyncConfigList{
  map<string,SyncConfig> items = 1;
}

message UserFileImportRequest {
  string clusterKey = 1;
  byte data = 2;
  destinationPath string = 3;
}

message ListSyncedConfigRequest{
  // cluster keys
  repeated string items = 1;
}

message ListImportedFiles{
  // cluster keys
  repeated string items = 1;
}
```

intended to be called from the CLI at least initially:

- `opni alerting sync <filename> --cluster <cluster-name or id>`

many production settings require setting explicit files:

- `opni alerting sync import <filename> --cluster <cluster-name> --destination-path`

<hr/>

- the current `AlertEndpoint` tag system (list of strings) will be replaced with Prometheus `label matchers`

Some additional notes:

- Syncing user configurations will return a validation error if the configuration requires files, for example CA certs, that aren't not found -- see above

<hr/d>

- Endpoints found in the sync process will be listed in the Endpoints UI, but will appear as **Read only** && **Synced** via labels
- Attempting to edit the endpoints through the admin UI will result in a `validation error` prompting the user to edit the configurations that have already synced directly instead.

<hr/>

- Conditions that have labels matching the endpoints will have these endpoints attached to them automatically, meaning Opni internal conditions will be able to use the synced endpoints

### UI

- Endpoint pages for each endpoint type
- Tags migrated to label matcher system with operations `==`,`!=`,`~=`,`~!=` for both endpoints & conditions

## Acceptance criteria:

- [ ] Reach endpoint parity with AlertManager by implementing Opni receiver functionality on remaining AlertManager receiver(s)
  - [ ] `Webhook`
  - [ ] `Amazon SNS`
  - [ ] `OpsGenie`
  - [ ] `Pushover`
  - [ ] `Victorops`
  - [ ] `Wechat`
  - [ ] `Telegram`
  - [ ] `Webex`
- [ ] Sync user configurations to Opni Router & Attach Cortex to them
- [ ] Import filesystem files for user configurations to Alerting Cluster
- [ ] Extend Opni Endpoint protocol buffer messages to include Prometheus `label matchers`
  - [] `==`, `!=`
  - [] `~=`, `~!=`
- [ ] Optimize `label matchers` when building Opni Router

## Supporting documents:

## Dependencies:

- https://github.com/rancher/opni/pull/942

## Risks and contingencies:

| Risk | Contingency |
| Some AlertManager endpoints may require premium licenses for functionality | Request license through SUSE |
| May include breaking changes to the AlertEndpoint protocol buffer message | Use existing opni alerting migration logic to migrate from one format to another |

## Level of Effort:

2 weeks

1 week : endpoint protocol buffers implementations
1 week : syncing user configurations

## Resources:

1 Upstream Opni Cluster

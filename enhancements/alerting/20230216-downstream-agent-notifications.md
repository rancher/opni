# Title:

Downstream Agent Notification System

## Summary:

Enable the downstream opni agents to push notifications to alerting's notification system.

For example, an interesting feature that requires this functionality is allowing SRE's to subscribe to
kubernetes event notifications in real time for their cluster

## Use case:

- Enable agents to send context clues about their cluster through alerting's notification system

## Benefits:

- Increased visibility into downstream clusters & downstream clusters right at the source

## Impact:

- Requires a (barebones) Alerting Plugin in downstream agents

## Implementation details:

- Implement an `AlertingBackend` alerting service for the gateway alerting plugin that implements the `capabilityv1.BackendServer`:

```proto
service Backend {
  // Returns info about the backend, including capability name
  rpc Info(google.protobuf.Empty) returns (Details);

  // Returns an error if installing the capability would fail.
  rpc CanInstall(google.protobuf.Empty) returns (google.protobuf.Empty) {
    option deprecated = true;
  }

  // Installs the capability on a cluster.
  rpc Install(InstallRequest) returns (InstallResponse);

  // Returns common runtime config info for this capability from a specific
  // cluster (node).
  rpc Status(StatusRequest) returns (NodeCapabilityStatus);

	// Requests the backend to clean up any resources it owns and prepare
	// for uninstallation. This process is asynchronous. The status of the
  // operation can be queried using the UninstallStatus method, or canceled
  // using the CancelUninstall method.
  rpc Uninstall(UninstallRequest) returns (google.protobuf.Empty);

  // Gets the status of the uninstall task for the given cluster.
  rpc UninstallStatus(core.Reference) returns (core.TaskStatus);

	// Cancels an uninstall task for the given cluster, if it is still pending.
  rpc CancelUninstall(core.Reference) returns (google.protobuf.Empty);

	// Returns a go template string which will generate a shell command used to
	// install the capability. This will be displayed to the user in the UI.
	// See InstallerTemplateSpec above for the available template fields.
  rpc InstallerTemplate(google.protobuf.Empty) returns (InstallerTemplateResponse) {
    option deprecated = true;
  }
}
```

- Implement an agent-side alerting plugin that implements the service `AlertingNode` which itself implements the `capabilityv1.NodeServer` and `controlv1.HealthServer`

```proto
service Node {
  rpc SyncNow(Filter) returns (google.protobuf.Empty) {
    option (totem.qos) = {
      replicationStrategy: Broadcast
    };
  }
}

service Health {
  rpc GetHealth(google.protobuf.Empty) returns (core.Health);
}
```

- Agent side plugin should implement the following minimalist apis for notifications

```proto

// agent-side applications consume this service to notify
service NotificationService{

  // preserves a priority queue of notifications, so that important notifications
  // are never lost to service communication errors
  rpc Notify(AgentNotification) returns (google.protobuf.Empty){}
}

// response-reply notification stream to gateway
service PushNotificationService {
  rpc Push(Notification) returns (google.protobuf.Empty){}
  // when this "signal" is received -- it is safe to purge that notification from the agent's queue
  rpc Resolve(core.Reference) returns (google.protobuf.Empty){}
}

message AgentNotification{
  // opaque id
  string id = 1;
  // same format as the gateway notification from OEP https://github.com/rancher/opni/blob/main/enhancements/alerting/20230131-alerting-msg-templating.md
  Notification message = 2;
}
```

- The `PushNotificationService` will be bidrectional

`NotificationService` is an additional abstraction layer that persists important notifications because :

- we want to preserve important notifications when alerting is not installed on the downstream
- we want resiliency in the case communication is interrupted between the gateway and agent

<hr/>

- The `NotificationService` must have an efficient priority queue for lookups, deletions & insertions as well as storage complexity :
  - Implement a `skiplist` datastructure for notification queueing

### UI/UX Changes

- Install alerting capability from Alerting admin install page, like other capabilities

## Acceptance criteria:

- [ ] Alerting agent capability
  - [ ] Alerting capability `Backend` service
  - [ ] Alerting agent plugin
    - [ ] capability `Node` service
    - [ ] control `Health` service
- [ ] Notification service for notifying through alerting gateway
- [ ] Agent notification service for queueing notifications

## Supporting documents:

N/A

## Dependencies:

Implementation of [messaging system](https://github.com/rancher/opni/blob/main/enhancements/alerting/20230124-messaging-system.md)

## Risks and contingencies:

| Risk | Contingency |
| ---- | ----------- |
| N/A  | N/A         |

## Level of Effort:

~ 1 1/2 weeks

- Barebones lightweight alerting plugin : 4 days
- Stream communication with Alerting gateway : 1-2 days
- Notification priority queueing in alerting plugin : 1-2 days

## Resources:

1 Upstream Opni cluster & 1 Downstream Opni cluster

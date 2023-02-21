# Title:

Alerting message templating

## Summary:

Opni Alerting needs to define a templating system for users to consume.
The templating system address the need to embed runtime or alarm specific information in the body or title of an Opni Alarm.

## Use case:

- Users need more information in their alerts in order to diagnose issues

## Benefits:

- Improved UX
- Improved time to resolution of Opni Alarms

## Impact:

- Opni-Alerting will need to deploy a `config map` with templates

## Implementation details:

- Each AlertManager receiver type would have their own template file

  - With Kubernetes drivers, mount a `config map` containing these template files into the Alerting Cluster

- `Plain` and `Alarm` messages should have different message templates, therefore each receiver template needs to condition expand to different templates based on the message type.
- AlertManager identifies the message type at dispatch time by AlertManager annotations on the incoming message.
- Opni-Alerting plugin determines the type of the message sent, not the caller of the respective APIs, for example `PushMessage` API or other.

### Alarms

- Each template file should expand template variables (using the Go template language)
- Each template file should come with a prebuilt header including:
  - ClusterId & ClusterName
  - Installed capabilities
  - Opni Alarm name
  - Status of the Alarm (firing, resolved, …) + (any string explanation of the state found)
  - Link-back URL to admin dashboard
- A `ListTemplateVariables` API will index the variables in the API files and template variables specific to the conditions
- condition-specific template variables slot into separate categories:
  - condition metadata : statically available through the condition definition. For example, `cluster-timeout` for agent disconnect alarms
  - `datasource` specific : grabs useful information from API calls to the respective backends. For example, for the metrics `datasource`, including the cortex cluster status or including a query value from cortex.
  - cluster metadata specific : information extracted from the management API

### Plain

- `Plain` messages sent from should include a header with the `backend`,for example AiOps service versus some other opni service, that's sending them the message.
- No template variables will be available for plain messages to consume.

## Acceptance criteria:

- [ ] Mount template files for each receiver definition
- [ ] Implement message formats for `Plain` and `Alarm`
- [ ] List static template variables from template files, for use by the `ListTemplateVariables` API
- [ ] Implement extension methods to `AlertCondition` protocol buffer which adds annotations to alarms based on condition metadata
- [ ] Implement management API annotations passed to alarms
  - [ ] Management event watcher should propagate updates to information in these annotations
- [ ] Implement `datasource` API annotations passed to alarms

## Supporting documents:

## Dependencies:

- Improved management broadcast event watcher : https://github.com/rancher/opni/pull/969
- Alerting Cluster as a messaging system : https://github.com/rancher/opni/pull/973

## Risks and contingencies:

| Risk                                                                                 | Contingency                 |
| ------------------------------------------------------------------------------------ | --------------------------- |
| Requires premium access to some receivers, like PagerDuty to test message templating | Request a license with SUSE |

## Level of effort:

2 days setting up template mounting system
2 days setting up API annotations
1/2 additional day per receiver, (3 as of writing this proposal, but up to ~10)
1 day documentation updates

## Resources:

1 Opni upstream cluster & 1 Opni downstream cluster

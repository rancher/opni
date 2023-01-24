# Title:

Alert Routing Integration with Metrics

## Summary:

Opni Alerting should allow for importing existing AlertManager configurations and make them work out of the box with both Cortex & Opni Alerting.

## Use case:

- Users have existing production-ready Prometheus AlertManager configurations they wish to use

## Benefits:

- Users retain existing production-grade functionality of user configurations
- Less configuration for experience users
- Use Opni-Alerting features with existing configurations
- Less overall user input to get started with Opni & Opni-Alerting

## Impact:

Won't impact existing Opni functionality, only allows for more targets.

## Implementation details:

WIP

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

3 days : Endpoint parity with AlertManager
1 day : Sync user configurations
2 days : Import filesystem files to AlertingCluster
2 days : Extend Opni Endpoint protocol buffer messages
2 days : Optimize `label matchers` when building Opni Router

## Resources:

1 Upstream Opni Cluster

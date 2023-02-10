# Title:

Prometheus Alerting Rule Extensions

## Summary:

Opni Alerting currently offers a means of creating custom Prometheus rules for evaluating alerts sent to the Alerting Cluster. To fully leverage the power of the abstractions built into Opni-Alerting, Opni-Alerting should provide enhancement options that apply to generic Prometheus rules.

More concretely, provide an abstraction for adding optional alerting rules derived from the user's custom Prometheus query, or, `Alert rule extensions`.

The following options can be considered `Alert rule extensions` :

- Alert if the rule evaluates to empty for X seconds
- Alert if the scrape source is disconnected for X seconds
- Alert if the scrape source is duplicated

## Use case:

- Users expect this functionality from a managed alerting & monitoring system

## Benefits:

- Remove the need for users to add error-prone generic embedded sub-queries to their Prometheus rule(s) altogether.
- Eliminates users wasting their time figuring out tricky edge cases that cause their metrics to fire alerts
- Give users additional information about why their alarms are firing

## Impact:

## Implementation details:

- Go interface for wrapping mutations to PromQL queries in a safe and predictable way
- OpniAlerting manages external dependencies by naming convention in the external source; this needs to change so that OpniAlerting `AlertConditions` track a list of dependencies by reference:

```proto
message AlertCondition {
  // ...
  repeated Reference deps = X;
}
```

```proto
message Reference {
   BackendType backend = 1; // BackendType_CortexRule
   string id = 2;
}
```

- Create PromQL wrapper expressions around Prometheus queries that create new Prometheus queries
  - Some query structures will require calling the promQl interpreter and checking the outmost promQL datatype (`instant`, `vec`, `matrix`, etc…)
- Store optional alerts in the same rule group as the Prometheus query alert.
- Status API will read the firing alerts and aggregate them into a single state, with a string explanation. For example `scrape source was lost`, `no data found`, etc…

## Acceptance criteria:

- [ ] Enhancements to the management of external dependencies by Opni-Alerting
- [ ] Implement abstractions over scrape metric(s)
- [ ] Implement empty rule extensions
- [ ] Implement scrape source rule extensions
- [ ] Extend `Status()` API to include a reason string, that indicates why the alarm is in the given state

## Supporting documents:

## Dependencies:

- Condition Status Optimizations https://github.com/rancher/opni/pull/971

## Risks and contingencies:

| Risk                                                                                           | Contingency                                                                                                                        |
| ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Ongoing risk on breaking scrape target extensions : scrape metrics have concrete metrics names | Building an abstraction in the metrics relabelling / scraping that ensures scrape metrics are consistently named to the same thing |

## Level of effort:

2 weeks

- 1 week implementing PromQL abstractions and dependency management
- 3 days status & alert rule extensions

## Resources:

1 Upstream & 1 Downstream opni cluster

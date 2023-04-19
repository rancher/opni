# Title:

Dependency management system for Alerting to use at runtime

## Summary:

Since alerting depends on external dependencies, that it doesn't necessarily have control over, therefore to evaluate alarms it
needs to have a way of managing external dependencies

## Use case:

- A generic and "type-safe" system for alarms depending on metric exporter libraries
- Enables implementing generic composite alarms : alarms requiring other alarms or alarms that require multiple opni capabilities or multiple external dependencies
- Intuitive visualization of how alarms connect to each other

## Benefits:

- When creating Alarms, only list the alarm types that are available for creation
- A "type-safe" system for alarms depending on metric exporter libraries
- Composite alarms (alarms requiring other alarms)
- Improved consistency of alarm evaluation based on external dependencies we can't necessarily control
- Less cognitive overhead & manual testing overhead for external dependencies

## Impact:

<!-- A description of how the proposed change or new feature would impact the existing system, including any potential trade-offs or drawbacks. -->

## Implementation details:

### Inner Graph Data format

- `gonum.DirectedMultiGraph` implementation as a property graph

  - non-leaf nodes represent a granular observable dependency
  - leaf nodes contain an Alarm Object
  - nodes contain a `task.Controller` which manages how updates to the node property should be propagated by children
  - A Controller acting on a particular alarm object requires a timeout for completion and will prevent other updates from APIs trying to modify that alarm object
  - when a new task is started in a parent node, it recursively cancels sub-tasks in children nodes
  - edges contain properties that evaluate w.r. to the observable dependency in the node (opaque labelling system that guarantees backwards compatibility & forward compatibility if we need to add new features to the graph, for example, tenancy)

- ensuring that whenever a node is added, reject any node that makes the graph cyclic
- an initial construction of the starting graph (for example, condition types that are themselves leaves in the graph) should panic if the graph is cyclic

The granular dependency is put behind an interface:

```
type Dependency interface {
    core.Referenceable
    GraphLookup() int32 // gonum.Graph requires int32 ids
    // ...
}
```

### Dependency Watcher

Abstraction

```go
// in pkg/apis/corev1
type Referenceable interface{
    GetId() *core.Reference
}

type Watcher[Cfg Referenceable] interface {
    Register[c Consumer[Cfg]]
}

type Consumer[Cfg Referenceable] interface {
    // Update is called by the watcher whenever a new dependency update is available
    Update(cfg Cfg)
}
```

- Alerting watcher system listens and applies dependency status updates to the graph. Graph delegates changes to alarms.

### APIs

- Explain APIs filter the graph by the given filters
- Returns a serialized graph representation : `svg`, `dot`, …
- we can generically handle different input formats using the `gonum/graph/Encoding` interface implementations

```
// This is our existing filter message on all alarm objects
message ListAlertConditionRequest {
  repeated string clusters = 1;
  repeated alerting.OpniSeverity severities = 2;
  repeated string labels = 3;
  repeated AlertType alertTypes = 4;
}

// should be a part of corev1 for topology compat
message Graph {
    string format = 1; // things like "svg", "dot", etc...
    bytes data = 2;
}

message ExplainRequest {
    ListAlertConditionRequest filter = 1;
    string format = 2;
}

message ExplainOneRequest {
    core.Reference alarmId = 1;
    string format = 2;
}

service DependencyManager{
  rpc Explain(ListAlertConditionRequest) returns (corev1.Graph) {
    option (google.api.http) = {
      post : "/explain"
      body : "*"
    };
  }

  rpc ExplainAlarm(ExplainOneRequest) returns (corev1.Graph) {
    option (google.api.http) = {
      post : "/explain/{id.Id}"
      body : "*"
    };
  }
}
```

## Acceptance criteria:

### Internal graph system

- [] [`gonum.Node`](https://pkg.go.dev/gonum.org/v1/gonum/graph#Node) implementation for encapsulating a granular observable dependency
- [] [`gonum.Edge`](https://pkg.go.dev/gonum.org/v1/gonum/graph#Edge) implementation for embedding properties to make this a ][property graph](https://en.wikipedia.org/wiki/Graph_database#Labeled-property_graph)
- [] [`gonum.DirectedMultigraph`](https://pkg.go.dev/gonum.org/v1/gonum/graph#DirectedMultigraph) implementation for the above nodes and edges
- [] [`gonum.DirectedMultigraphBuilder`](https://pkg.go.dev/gonum.org/v1/gonum/graph#DirectedMultigraphBuilder) implementation for preventing cyclic additions
- [] Add `task.Controller` to nodes, doing a traversal on children running their own `task.Controller`

### Watcher System

- [] the `Dependency` in each non-terminal interface has `Wachter` & `Consumer` interface

### Explain APIs

- [] Dependency Graph traversal by alerting filters
- [] Graph serialization, initially support two formats `svg` & `dot`

### Misc API changes

- [] APIs that modify alarm objects check if dependency graph has a controller running a task on said alarm object
- [] ListAlarmChoices only lists alarm choices for available dependencies
- [] Label any alarm that does not meet the required dependencies `Invalidated`

## Supporting documents:

- [property graph](https://en.wikipedia.org/wiki/Graph_database#Labeled-property_graph)
- [`gonum`](https://pkg.go.dev/gonum.org/v1/gonum)
- [`gonum` graph](https://pkg.go.dev/gonum.org/v1/gonum/graph)
- [`gonum` graph serialization](https://pkg.go.dev/gonum.org/v1/gonum/graph/encoding)
- [`Opni task.Controller`](https://github.com/rancher/opni/blob/e128a9ee122dec86e055cbc763d8184a77b6f1cf/pkg/task/controller.go#L27)

## Dependencies:

- N/A

## Risks and contingencies:

- N/A

## Level of Effort:

~3 weeks

- 1 1/2 week for Graph acceptance criteria
- 1/2 week for Watcher acceptance criteria
- 1 week remaining APIs acceptance criteria & testing

## Resources:

1 Opni upstream cluster & 1 Opni downstream cluster

# Title: Readable Logging Name

## Summary:

Currently, when users select an agent cluster for log analytics, they select from a list of cluster IDs. This makes it difficult to select the right agent when many are registered. This proposal aims to add human-readable agent cluster names, to help differentiate the cluster names for the user.

## Use case:

When an Opni Gateway has many connected agents, it becomes cumbersome to search through a list of IDs to find the one which matches the target agent.

## Benefits:

By viewing a list of human-readable names, it will be immediately clear which cluster you are selecting. While the IDs do uniquely identify each agent, it's not immediately clear what responsibilities each agent has. Human-readable names will make agent responsibility clearer, as well as being easier to remember.

## Impact:

## Implementation details:

When a new logging cluster is registered with Opni, the gateway will need to send a request to `logging/agent` to inform it of the new logging agent details as described below.

### Agent Details Service
First we will add agent details endpoints to the `LoggingAdminV2` service, to expose an internal mapping of cluster ids and details:

```protobuf
syntax = "proto3";

service LoggingAdminV2 {

  // existing rpc services...

  rpc CreateOrUpdateAgentDetails(ClusterDetails) returns(google.protobuf.Empty) {
    option (google.api.http) = {
      get: "/logging/agent"
    };
  }

  rpc GetAgentDetails(DetailsRequest) returns(AgentDetailsList) {
    option (google.api.http) = {
      get: "/logging/agent/details"
    };
  }
}

message DetailsRequest {
  string namePattern = 1;
}

message RenameRequest {
  string oldName = 2;
  string newName = 3;
}

message AgentDetailsList {
  repeated ClusterDetails names = 1;
}

enum AgentState {
  Unknown = 0;
  Active = 1;
  Inactive = 2;
}

message AgentDetails {
  string id = 1;
  string name = 2;
  AgentState = 3;
}
```

Internally, the `LoggingManagerV2` will  need to store the agent cluster details to give access to that information:

```golang
type LoggingManagerV2 struct {
	loggingadmin.UnsafeLoggingAdminV2Server
	k8sClient         client.Client
	logger            *zap.SugaredLogger
	opensearchCluster *opnimeta.OpensearchClusterRef
	opensearchManager *opensearchdata.Manager
	storageNamespace  string
	natsRef           *corev1.LocalObjectReference
	versionOverride   string
	
	// mapping of agent ids to agent details
	agentDetails map[string]*loggingadmin.AgentDetails
}
```

### OpenSearch

To allow users to query logs using the readable name, we will leverage [Join Fields Types](https://opensearch.org/docs/latest/opensearch/supported-field-types/join/) to help manage the relationship between the log documents and their related agent. This approach will avoid the need to update every document for the renamed agent. To create this relationship:

```
PUT opni_log_index
{
  "mappings": {
    "properties": {
      "log_to_agent": {
        "type": "join",
        "relationship": {
          "agent": "doc"
        }
      }
    }
  }
}
```

#### Adding Agent
When agents are registered to Opni:

```
PUT opni_log_index/_doc/1
{
  "name": "0194fdc2-fa2f-4cc0-81d3-ff12045b73c8",
  "log_to_agent": {
    "name": "agent"
  }
}

PUT opni_log_index/_doc/2
{
  "name": "6e4ff95f-f662-45ee-a82a-bdf44a2d0b75",
  "log_to_agent": {
    "name": "agent"
  }
}
```

#### Adding Logs
When sending logs to OpenSearch we just need to add the join field:

```
PUT opni_log_index
{
  // log fields...
  "log_to_agent": {
    "name": "log",
    "parent": "1",  // the document id for the 0194fdc2-fa2f-4cc0-81d3-ff12045b73c8 parent
  }
}
```

#### Rename Agent

When the user renames an agent, Opni will need to edit the parent document for that agent. For example, if the user renames agent `0194fdc2-fa2f-4cc0-81d3-ff12045b73c8` to `renameed-agent`:

```
POST oni_log_index/_update/1
{
  "doc": {
    "name": "renamed-agent"
  }
}
```

#### Querying

When querying for agent logs, the user would send a query for children of that parent:

```
GET opni_log_index/_search
{
  "query": {
    "has_parent": {
      "paernt_type": "agent",
      "query": {
        "match": {
          "name": "renamed-agent"
        }
      }
    }
  }
}
```

## Acceptance criteria:

- [ ] Running `opni cluster rename <cluster> <new-name>` should:
  - [ ] Update the logging manager agent details store
  - [ ] Update OpenSearch parent document
- [ ] User should be able to view the new name under the cluster dropdown
- [ ] User should be able to query OpenSearch for agent logs using the new name

## Supporting documents:

## Dependencies:

## Risks and contingencies:

## Level of Effort:

This addition should take 2 weeks for implementation and testing

## Resources:

The additional resource consumption will be negligible
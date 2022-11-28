package shared

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Jetstream streams
const (
	// global streams
	AgentClusterHealthStatusStream        = "agent-cluster-health-status"
	AgentClusterHealthStatusSubjects      = "agent-cluster-health-status.*"
	AgentClusterHealthStatusDurableReplay = "agent-cluster-health-status-consumer"

	//streams
	AgentDisconnectStream         = "opni_alerting_agent"
	AgentDisconnectStreamSubjects = "opni_alerting_agent.*"
	AgentHealthStream             = "opni_alerting_health"
	AgentHealthStreamSubjects     = "opni_alerting_health.*"
	CortexStatusStream            = "opni_alerting_cortex_status"
	CortexStatusStreamSubjects    = "opni_alerting_cortex_status.*"
	// buckets
	AgentDisconnectBucket              = "opni-alerting-agent-bucket"
	AgentStatusBucket                  = "opni-alerting-agent-status-bucket"
	StatusBucketPerCondition           = "opni-alerting-condition-status-bucket"
	StatusBucketPerClusterInternalType = "opni-alerting-cluster-condition-type-status-bucket"
	GeneralIncidentStorage             = "opni-alerting-general-incident-bucket"
)

func NewGlobalAgentClusterHealthStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      AgentClusterHealthStatusStream,
		Subjects:  []string{AgentClusterHealthStatusSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB
	}
}

func NewGlobalAgentClusterHealthDurableReplayConsumer() *nats.ConsumerConfig {
	return &nats.ConsumerConfig{
		Durable: AgentClusterHealthStatusDurableReplay,
		//AckPolicy:      nats.AckExplicitPolicy,
		ReplayPolicy: nats.ReplayOriginalPolicy,
	}
}

func NewAlertingDisconnectStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      AgentDisconnectStream,
		Subjects:  []string{AgentDisconnectStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB (allocation for all agents)
	}
}

func NewAlertingHealthStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      AgentHealthStream,
		Subjects:  []string{AgentHealthStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB (allocation for all agents)
	}
}

func NewCortexStatusStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      CortexStatusStream,
		Subjects:  []string{CortexStatusStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB
	}
}

func NewCortexStatusSubject() string {
	return fmt.Sprintf("%s.%s", CortexStatusStream, "cortex")
}

func NewAgentDisconnectSubject(agentId string) string {
	return fmt.Sprintf("%s.%s", AgentDisconnectStream, agentId)
}

func NewHealthStatusSubject(clusterId string) string {
	return fmt.Sprintf("%s.%s", AgentHealthStream, clusterId)
}

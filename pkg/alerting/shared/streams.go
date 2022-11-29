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

func NewAgentStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      AgentClusterHealthStatusStream,
		Subjects:  []string{AgentClusterHealthStatusSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB
	}
}

func NewAgentDurableReplayConsumer(clusterId string) *nats.ConsumerConfig {
	return &nats.ConsumerConfig{
		Durable:        NewDurableAgentReplaySubject(clusterId),
		DeliverSubject: NewDurableAgentReplaySubject(clusterId),
		DeliverPolicy:  nats.DeliverNewPolicy,
		FilterSubject:  NewAgentStreamSubject(clusterId),
		AckPolicy:      nats.AckExplicitPolicy,
		ReplayPolicy:   nats.ReplayInstantPolicy,
	}
}

func NewDurableAgentReplaySubject(clusterId string) string {
	return fmt.Sprintf("%s-%s", AgentClusterHealthStatusDurableReplay, clusterId)
}

func NewAgentStreamSubject(clusterId string) string {
	return fmt.Sprintf("%s.%s", AgentClusterHealthStatusStream, clusterId)
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

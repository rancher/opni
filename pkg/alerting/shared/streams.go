package shared

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Jetstream streams
const (
	AgentDisconnectStream         = "opni_alerting_agent"
	AgentDisconnectStreamSubjects = "opni_alerting_agent.*"
	AgentDisconnectBucket         = "opni-alerting-agent-bucket"
)

func NewAlertingDisconnectStream() *nats.StreamConfig {
	return &nats.StreamConfig{
		Name:      AgentDisconnectStream,
		Subjects:  []string{AgentDisconnectStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
		MaxBytes:  1 * 1024 * 50, //50KB (allocation for all agents)
	}
}

func NewAgentDisconnectSubject(agentId string) string {
	return fmt.Sprintf("%s.%s", AgentDisconnectStream, agentId)
}

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

// blocking
func NewAlertingDisconnectStream(mgr nats.JetStreamContext) error {
	if alertingStream, _ := mgr.StreamInfo(AgentDisconnectStream); alertingStream == nil {
		_, err := mgr.AddStream(&nats.StreamConfig{
			Name:      AgentDisconnectStream,
			Subjects:  []string{AgentDisconnectStreamSubjects},
			Retention: nats.LimitsPolicy,
			MaxAge:    1 * time.Hour,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func NewAgentDisconnectSubject(agentId string) string {
	return fmt.Sprintf("%s.%s", AgentDisconnectStream, agentId)
}

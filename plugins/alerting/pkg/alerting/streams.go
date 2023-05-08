package alerting

// import (
// 	"fmt"
// 	"time"

// 	"github.com/nats-io/nats.go"
// 	"github.com/rancher/opni/pkg/alerting/shared"
// )

// func NewAgentStream() *nats.StreamConfig {
// 	return &nats.StreamConfig{
// 		Name:      shared.AgentClusterHealthStatusStream,
// 		Subjects:  []string{shared.AgentClusterHealthStatusSubjects},
// 		Retention: nats.LimitsPolicy,
// 		MaxAge:    1 * time.Hour,
// 		MaxBytes:  1 * 1024 * 50, //50KB
// 	}
// }

// func NewAgentDurableReplayConsumer(clusterId string) *nats.ConsumerConfig {
// 	return &nats.ConsumerConfig{
// 		Durable:        NewDurableAgentReplaySubject(clusterId),
// 		DeliverSubject: NewDurableAgentReplaySubject(clusterId),
// 		DeliverPolicy:  nats.DeliverNewPolicy,
// 		FilterSubject:  NewAgentStreamSubject(clusterId),
// 		AckPolicy:      nats.AckExplicitPolicy,
// 		ReplayPolicy:   nats.ReplayInstantPolicy,
// 	}
// }

// func NewDurableAgentReplaySubject(clusterId string) string {
// 	return fmt.Sprintf("%s-%s", shared.AgentClusterHealthStatusDurableReplay, clusterId)
// }

// func NewAgentStreamSubject(clusterId string) string {
// 	return fmt.Sprintf("%s.%s", shared.AgentClusterHealthStatusStream, clusterId)
// }

// func NewCortexStatusStream() *nats.StreamConfig {
// 	return &nats.StreamConfig{
// 		Name:      shared.CortexStatusStream,
// 		Subjects:  []string{shared.CortexStatusStreamSubjects},
// 		Retention: nats.LimitsPolicy,
// 		MaxAge:    1 * time.Hour,
// 		MaxBytes:  1 * 1024 * 50, //50KB
// 	}
// }

// func NewCortexStatusSubject() string {
// 	return fmt.Sprintf("%s.%s", shared.CortexStatusStream, "cortex")
// }

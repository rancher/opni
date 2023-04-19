package otel

import "fmt"

const (
	CollectorName = "opni"
)

func AgentEndpoint(serviceName string) string {
	return fmt.Sprintf("http://%s/api/agent/otel", serviceName)
}

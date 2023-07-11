package wellknown

const (
	CapabilityLogs     = "logs"
	CapabilityMetrics  = "metrics"
	CapabilityTraces   = "traces"
	CapabilityAlerting = "alerting"
	CapabilityTopology = "topology"
	CapabilityExample  = "example"
)

func KnownCapabilities() []string {
	return []string{
		CapabilityAlerting,
		CapabilityMetrics,
		CapabilityLogs,
		CapabilityTraces,
		CapabilityTopology,
	}
}

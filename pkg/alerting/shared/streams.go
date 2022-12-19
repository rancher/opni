package shared

// Jetstream streams
const (
	// global streams
	AgentClusterHealthStatusStream        = "agent-cluster-health-status"
	AgentClusterHealthStatusSubjects      = "agent-cluster-health-status.*"
	AgentClusterHealthStatusDurableReplay = "agent-cluster-health-status-consumer"

	//streams
	AgentDisconnectStream             = "opni_alerting_agent"
	AgentDisconnectStreamSubjects     = "opni_alerting_agent.*"
	AgentHealthStream                 = "opni_alerting_health"
	AgentHealthStreamSubjects         = "opni_alerting_health.*"
	CortexStatusStream                = "opni_alerting_cortex_status"
	CortexStatusStreamSubjects        = "opni_alerting_cortex_status.*"
	ModelTrainingStatusStream         = "opni_alerting_model_training_status"
	ModelTrainingStatusStreamSubjects = "opni_alerting_model_training_status.*"
	// buckets
	AgentDisconnectBucket              = "opni-alerting-agent-bucket"
	AgentStatusBucket                  = "opni-alerting-agent-status-bucket"
	StatusBucketPerCondition           = "opni-alerting-condition-status-bucket"
	StatusBucketPerClusterInternalType = "opni-alerting-cluster-condition-type-status-bucket"
	GeneralIncidentStorage             = "opni-alerting-general-incident-bucket"
)

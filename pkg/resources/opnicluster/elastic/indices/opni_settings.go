package indices

import (
	"fmt"

	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"

	_ "embed" // embed should be a blank import
)

const (
	LogPolicyName                = "log-policy"
	LogIndexPrefix               = "logs-v0.5.4"
	LogIndexAlias                = "logs"
	LogIndexTemplateName         = "logs_rollover_mapping"
	logTemplatePolicyName        = "template-policy"
	logTemplateIndexPrefix       = "templates-v0.5.4"
	logTemplateIndexAlias        = "templates"
	logTemplateIndexTemplateName = "templates_rollover_mapping"
	PreProcessingPipelineName    = "opni-ingest-pipeline"
	drainStatusPolicyName        = "opni-drain-model-status-policy"
	drainStatusIndexPrefix       = "opni-drain-model-status-v0.1.3"
	drainStatusIndexAlias        = "opni-drain-model-status"
	drainStatusIndexTemplateName = "opni-drain-model-status_rollover_mapping"
	metricPolicyName             = "opni-metric-policy"
	metricIndexPrefix            = "opni-metric-v0.3.0"
	metricIndexAlias             = "opni-metric"
	metricIndexTemplateName      = "opni-metric_rollover_mapping"
	normalIntervalIndexName      = "opni-normal-intervals"
	kibanaDashboardVersionDocID  = "latest"
	kibanaDashboardVersion       = "v0.5.4"
	kibanaDashboardVersionIndex  = "opni-dashboard-version"
)

var (
	OldIndexPrefixes = []string{
		"logs-v0.1.3*",
		"logs-v0.5.1*",
	}
	DefaultRetry = osapiext.RetrySpec{
		Count:   3,
		Backoff: "exponential",
		Delay:   "1m",
	}
	OpniLogPolicy = osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   LogPolicyName,
			MarshallID: false,
		},
		Description:  "Opni policy with hot-warm-cold workflow",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinIndexAge: "1d",
								MinSize:     "20gb",
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "2d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", LogIndexPrefix),
				},
				Priority: 100,
			},
		},
	}

	oldOpniLogPolicy = osapiext.OldISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   LogPolicyName,
			MarshallID: false,
		},
		Description:  "Opni policy with hot-warm-cold workflow",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinIndexAge: "1d",
								MinSize:     "20gb",
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "2d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: &osapiext.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", LogIndexPrefix),
			},
			Priority: 100,
		},
	}
	opniLogTemplatePolicy = osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   logTemplatePolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the templates index.",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", logTemplateIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
	opniDrainModelStatusPolicy = osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", drainStatusIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
	oldOpniDrainModelStatusPolicy = osapiext.OldISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: &osapiext.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", drainStatusIndexPrefix),
			},
			Priority: 100,
		},
	}

	opniMetricPolicy = osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   metricPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-metric index.",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", metricIndexPrefix),
				},
				Priority: 100,
			},
		},
	}

	oldOpniMetricPolicy = osapiext.OldISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   metricPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-metric index.",
		DefaultState: "hot",
		States: []osapiext.StateSpec{
			{
				Name: "hot",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Rollover: &osapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReplicaCount: &osapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							ReadOnly: &osapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []osapiext.ActionSpec{
					{
						ActionOperation: &osapiext.ActionOperation{
							Delete: &osapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: &osapiext.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", metricIndexPrefix),
			},
			Priority: 100,
		},
	}

	OpniLogTemplate = osapiext.IndexTemplateSpec{
		TemplateName: LogIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", LogIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   1,
				NumberOfReplicas: 1,
				ISMPolicyID:      LogPolicyName,
				RolloverAlias:    LogIndexAlias,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				Properties: map[string]osapiext.PropertySettings{
					"timestamp": {
						Type: "date",
					},
					"time": {
						Type: "date",
					},
					"log": {
						Type: "text",
					},
					"masked_log": {
						Type: "text",
					},
					"log_type": {
						Type: "keyword",
					},
					"kubernetes_component": {
						Type: "keyword",
					},
					"cluster_id": {
						Type: "keyword",
					},
					"anomaly_level": {
						Type: "keyword",
					},
				},
			},
		},
		Priority: 100,
	}

	IngestPipelineTemplate = osapiext.IndexTemplateSpec{
		TemplateName: "logs-ingest-pipeline",
		IndexPatterns: []string{
			fmt.Sprintf("%s*", LogIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				DefaultPipeline: PreProcessingPipelineName,
			},
		},
		Priority: 50,
	}

	opniMetricTemplate = osapiext.IndexTemplateSpec{
		TemplateName: metricIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", metricIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      metricPolicyName,
				RolloverAlias:    metricIndexAlias,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				Properties: map[string]osapiext.PropertySettings{
					"timestamp": {
						Type: "date",
					},
				},
			},
		},
	}

	drainStatusTemplate = osapiext.IndexTemplateSpec{
		TemplateName: drainStatusIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", drainStatusIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      drainStatusPolicyName,
				RolloverAlias:    drainStatusIndexAlias,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				Properties: map[string]osapiext.PropertySettings{
					"num_log_clusters": {
						Type: "integer",
					},
					"update_type": {
						Type: "keyword",
					},
					"timestamp": {
						Type:   "date",
						Format: "epoch_millis",
					},
				},
			},
		},
	}

	logTemplate = osapiext.IndexTemplateSpec{
		TemplateName: logTemplateIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", logTemplateIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      logTemplatePolicyName,
				RolloverAlias:    logTemplateIndexAlias,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				Properties: map[string]osapiext.PropertySettings{
					"log": {
						Type: "text"
					},
					"template_matched": {
						Type: "keyword"
					}
					"template_cluster_id": {
						Type: "integer",
					},
				},
			},
		},
	}


	normalIntervalIndexSettings = map[string]osapiext.TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]osapiext.PropertySettings{
				"start_ts": {
					Type:   "date",
					Format: "epoch_millis",
				},
				"end_ts": {
					Type:   "date",
					Format: "epoch_millis",
				},
			},
		},
	}

	PreprocessingPipeline = osapiext.IngestPipeline{
		Description: "Opni preprocessing ingest pipeline",
		Processors: []osapiext.Processor{
			{
				OpniPreProcessor: &osapiext.OpniPreProcessor{
					Field:       "log",
					TargetField: "masked_log",
				},
			},
		},
	}

	// kibanaObjects contains the ndjson form data for creating the kibana
	// index patterns and dashboards
	//go:embed dashboard.ndjson
	kibanaObjects string
)

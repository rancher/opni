package indices

import (
	"fmt"

	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
)

const (
	logTemplateIndexName         = "templates"
	drainStatusPolicyName        = "opni-drain-model-status-policy"
	drainStatusIndexPrefix       = "opni-drain-model-status-v0.1.3"
	drainStatusIndexAlias        = "opni-drain-model-status"
	drainStatusIndexTemplateName = "opni-drain-model-status_rollover_mapping"
	metricPolicyName             = "opni-metric-policy"
	metricIndexPrefix            = "opni-metric-v0.3.0"
	metricIndexAlias             = "opni-metric"
	metricIndexTemplateName      = "opni-metric_rollover_mapping"
	normalIntervalIndexName      = "opni-normal-intervals"
)

var (
	DefaultRetry = osapiext.RetrySpec{
		Count:   3,
		Backoff: "exponential",
		Delay:   "1m",
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

	logTemplateIndexSettings = map[string]osapiext.TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]osapiext.PropertySettings{
				"log": {
					Type: "text",
				},
				"template_matched": {
					Type: "keyword",
				},
				"template_cluster_id": {
					Type: "integer",
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
)

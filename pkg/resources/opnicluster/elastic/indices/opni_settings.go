package indices

import (
	"fmt"

	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
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
	DefaultRetry = opensearchtypes.RetrySpec{
		Count:   3,
		Backoff: "exponential",
		Delay:   "1m",
	}

	opniDrainModelStatusPolicy = opensearchtypes.ISMPolicySpec{
		ISMPolicyIDSpec: &opensearchtypes.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []opensearchtypes.StateSpec{
			{
				Name: "hot",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Rollover: &opensearchtypes.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReplicaCount: &opensearchtypes.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							IndexPriority: &opensearchtypes.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ForceMerge: &opensearchtypes.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReadOnly: &opensearchtypes.ReadOnlyOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Delete: &opensearchtypes.DeleteOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: make([]opensearchtypes.TransitionSpec, 0),
			},
		},
		ISMTemplate: []opensearchtypes.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", drainStatusIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
	oldOpniDrainModelStatusPolicy = opensearchtypes.OldISMPolicySpec{
		ISMPolicyIDSpec: &opensearchtypes.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []opensearchtypes.StateSpec{
			{
				Name: "hot",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Rollover: &opensearchtypes.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReplicaCount: &opensearchtypes.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							IndexPriority: &opensearchtypes.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ForceMerge: &opensearchtypes.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReadOnly: &opensearchtypes.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Delete: &opensearchtypes.DeleteOperation{},
						},
					},
				},
				Transitions: make([]opensearchtypes.TransitionSpec, 0),
			},
		},
		ISMTemplate: &opensearchtypes.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", drainStatusIndexPrefix),
			},
			Priority: 100,
		},
	}

	opniMetricPolicy = opensearchtypes.ISMPolicySpec{
		ISMPolicyIDSpec: &opensearchtypes.ISMPolicyIDSpec{
			PolicyID:   metricPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-metric index.",
		DefaultState: "hot",
		States: []opensearchtypes.StateSpec{
			{
				Name: "hot",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Rollover: &opensearchtypes.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReplicaCount: &opensearchtypes.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							IndexPriority: &opensearchtypes.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &DefaultRetry,
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ForceMerge: &opensearchtypes.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReadOnly: &opensearchtypes.ReadOnlyOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Delete: &opensearchtypes.DeleteOperation{},
						},
						Retry: &DefaultRetry,
					},
				},
				Transitions: make([]opensearchtypes.TransitionSpec, 0),
			},
		},
		ISMTemplate: []opensearchtypes.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", metricIndexPrefix),
				},
				Priority: 100,
			},
		},
	}

	oldOpniMetricPolicy = opensearchtypes.OldISMPolicySpec{
		ISMPolicyIDSpec: &opensearchtypes.ISMPolicyIDSpec{
			PolicyID:   metricPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-metric index.",
		DefaultState: "hot",
		States: []opensearchtypes.StateSpec{
			{
				Name: "hot",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Rollover: &opensearchtypes.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReplicaCount: &opensearchtypes.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							IndexPriority: &opensearchtypes.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ForceMerge: &opensearchtypes.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							ReadOnly: &opensearchtypes.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []opensearchtypes.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &opensearchtypes.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []opensearchtypes.ActionSpec{
					{
						ActionOperation: &opensearchtypes.ActionOperation{
							Delete: &opensearchtypes.DeleteOperation{},
						},
					},
				},
				Transitions: make([]opensearchtypes.TransitionSpec, 0),
			},
		},
		ISMTemplate: &opensearchtypes.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", metricIndexPrefix),
			},
			Priority: 100,
		},
	}

	opniMetricTemplate = opensearchtypes.IndexTemplateSpec{
		TemplateName: metricIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", metricIndexPrefix),
		},
		Template: opensearchtypes.TemplateSpec{
			Settings: opensearchtypes.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      metricPolicyName,
				RolloverAlias:    metricIndexAlias,
			},
			Mappings: opensearchtypes.TemplateMappingsSpec{
				Properties: map[string]opensearchtypes.PropertySettings{
					"timestamp": {
						Type: "date",
					},
				},
			},
		},
	}

	drainStatusTemplate = opensearchtypes.IndexTemplateSpec{
		TemplateName: drainStatusIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", drainStatusIndexPrefix),
		},
		Template: opensearchtypes.TemplateSpec{
			Settings: opensearchtypes.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      drainStatusPolicyName,
				RolloverAlias:    drainStatusIndexAlias,
			},
			Mappings: opensearchtypes.TemplateMappingsSpec{
				Properties: map[string]opensearchtypes.PropertySettings{
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

	logTemplateIndexSettings = map[string]opensearchtypes.TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]opensearchtypes.PropertySettings{
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

	normalIntervalIndexSettings = map[string]opensearchtypes.TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]opensearchtypes.PropertySettings{
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

package indices

import (
	"fmt"

	. "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"
)

const (
	logPolicyName           = "log-policy"
	logIndexPrefix          = "logs-v0.1.3"
	logIndexAlias           = "logs"
	drainStatusPolicyName   = "opni-drain-model-status-policy"
	drainStatusIndexPrefix  = "opni-drain-model-status-v0.1.3"
	drainStatusIndexAlias   = "opni-drain-model-status"
	normalIntervalIndexName = "opni-normal-intervals"
)

var (
	opniLogPolicy = ISMPolicySpec{
		PolicyId:     logPolicyName,
		Description:  "Opni policy with hot-warm-cold workflow",
		DefaultState: "hot",
		States: []StateSpec{
			{
				Name: "hot",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							Rollover: &RolloverOperation{
								MinIndexAge: "1d",
								MinSize:     "20gb",
							},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							ReplicaCount: &ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &ActionOperation{
							IndexPriority: &IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &ActionOperation{
							ForceMerge: &ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "cold",
						Conditions: &ConditionSpec{
							MinIndexAge: "2d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							ReadOnly: &ReadOnlyOperation{},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "delete",
						Conditions: &ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							Delete: &DeleteOperation{},
						},
					},
				},
				Transitions: make([]TransitionSpec, 0),
			},
		},
		ISMTemplate: &ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", logIndexPrefix),
			},
			Priority: 100,
		},
	}
	opniDrainModelStatusPolicy = ISMPolicySpec{
		PolicyId:     drainStatusPolicyName,
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []StateSpec{
			{
				Name: "hot",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							Rollover: &RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							ReplicaCount: &ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &ActionOperation{
							IndexPriority: &IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &ActionOperation{
							ForceMerge: &ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "cold",
						Conditions: &ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							ReadOnly: &ReadOnlyOperation{},
						},
					},
				},
				Transitions: []TransitionSpec{
					{
						StateName: "delete",
						Conditions: &ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []ActionSpec{
					{
						ActionOperation: &ActionOperation{
							Delete: &DeleteOperation{},
						},
					},
				},
				Transitions: make([]TransitionSpec, 0),
			},
		},
		ISMTemplate: &ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", drainStatusIndexPrefix),
			},
			Priority: 100,
		},
	}

	opniLogTemplate = IndexTemplateSpec{
		IndexPatterns: []string{
			fmt.Sprintf("%s*", logIndexPrefix),
		},
		Template: TemplateSpec{
			Settings: TemplateSettingsSpec{
				NumberOfShards:   1,
				NumberOfReplicas: 1,
				ISMPolicyID:      logPolicyName,
				RolloverAlias:    logIndexAlias,
			},
			Mappings: TemplateMappingsSpec{
				Properties: map[string]PropertySettings{
					"timestamp": {
						Type: "date",
					},
				},
			},
		},
	}
	drainStatusTemplate = IndexTemplateSpec{
		IndexPatterns: []string{
			fmt.Sprintf("%s*", drainStatusIndexPrefix),
		},
		Template: TemplateSpec{
			Settings: TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      drainStatusPolicyName,
				RolloverAlias:    drainStatusIndexAlias,
			},
			Mappings: TemplateMappingsSpec{
				Properties: map[string]PropertySettings{
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

	normalIntervalIndexSettings = map[string]TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]PropertySettings{
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

package indices

import (
	"fmt"

	esapiext "github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices/types"

	_ "embed" // embed should be a blank import
)

const (
	logPolicyName                = "log-policy"
	logIndexPrefix               = "logs-v0.1.3"
	logIndexAlias                = "logs"
	logIndexTemplateName         = "logs_rollover_mapping"
	drainStatusPolicyName        = "opni-drain-model-status-policy"
	drainStatusIndexPrefix       = "opni-drain-model-status-v0.1.3"
	drainStatusIndexAlias        = "opni-drain-model-status"
	drainStatusIndexTemplateName = "opni-drain-model-status_rollover_mapping"
	normalIntervalIndexName      = "opni-normal-intervals"
	kibanaDashboardVersionDocID  = "latest"
	kibanaDashboardVersion       = "v0.1.3"
	kibanaDashboardVersionIndex  = "opni-dashboard-version"
)

var (
	opniLogPolicy = esapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &esapiext.ISMPolicyIDSpec{
			PolicyID:   logPolicyName,
			MarshallID: false,
		},
		Description:  "Opni policy with hot-warm-cold workflow",
		DefaultState: "hot",
		States: []esapiext.StateSpec{
			{
				Name: "hot",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Rollover: &esapiext.RolloverOperation{
								MinIndexAge: "1d",
								MinSize:     "20gb",
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReplicaCount: &esapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							IndexPriority: &esapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							ForceMerge: &esapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "2d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReadOnly: &esapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Delete: &esapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]esapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []esapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", logIndexPrefix),
				},
				Priority: 100,
			},
		},
	}

	oldOpniLogPolicy = esapiext.OldISMPolicySpec{
		ISMPolicyIDSpec: &esapiext.ISMPolicyIDSpec{
			PolicyID:   logPolicyName,
			MarshallID: false,
		},
		Description:  "Opni policy with hot-warm-cold workflow",
		DefaultState: "hot",
		States: []esapiext.StateSpec{
			{
				Name: "hot",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Rollover: &esapiext.RolloverOperation{
								MinIndexAge: "1d",
								MinSize:     "20gb",
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReplicaCount: &esapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							IndexPriority: &esapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							ForceMerge: &esapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "2d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReadOnly: &esapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "7d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Delete: &esapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]esapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: &esapiext.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", logIndexPrefix),
			},
			Priority: 100,
		},
	}
	opniDrainModelStatusPolicy = esapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &esapiext.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []esapiext.StateSpec{
			{
				Name: "hot",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Rollover: &esapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReplicaCount: &esapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							IndexPriority: &esapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							ForceMerge: &esapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReadOnly: &esapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Delete: &esapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]esapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []esapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", drainStatusIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
	oldOpniDrainModelStatusPolicy = esapiext.OldISMPolicySpec{
		ISMPolicyIDSpec: &esapiext.ISMPolicyIDSpec{
			PolicyID:   drainStatusPolicyName,
			MarshallID: false,
		},
		Description:  "A hot-warm-cold-delete workflow for the opni-drain-model-status index.",
		DefaultState: "hot",
		States: []esapiext.StateSpec{
			{
				Name: "hot",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Rollover: &esapiext.RolloverOperation{
								MinSize:     "1gb",
								MinIndexAge: "1d",
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "warm",
					},
				},
			},
			{
				Name: "warm",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReplicaCount: &esapiext.ReplicaCountOperation{
								NumberOfReplicas: 0,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							IndexPriority: &esapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
					},
					{
						ActionOperation: &esapiext.ActionOperation{
							ForceMerge: &esapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "cold",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "5d",
						},
					},
				},
			},
			{
				Name: "cold",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							ReadOnly: &esapiext.ReadOnlyOperation{},
						},
					},
				},
				Transitions: []esapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &esapiext.ConditionSpec{
							MinIndexAge: "30d",
						},
					},
				},
			},
			{
				Name: "delete",
				Actions: []esapiext.ActionSpec{
					{
						ActionOperation: &esapiext.ActionOperation{
							Delete: &esapiext.DeleteOperation{},
						},
					},
				},
				Transitions: make([]esapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: &esapiext.ISMTemplateSpec{
			IndexPatterns: []string{
				fmt.Sprintf("%s*", drainStatusIndexPrefix),
			},
			Priority: 100,
		},
	}

	opniLogTemplate = esapiext.IndexTemplateSpec{
		TemplateName: logIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", logIndexPrefix),
		},
		Template: esapiext.TemplateSpec{
			Settings: esapiext.TemplateSettingsSpec{
				NumberOfShards:   1,
				NumberOfReplicas: 1,
				ISMPolicyID:      logPolicyName,
				RolloverAlias:    logIndexAlias,
			},
			Mappings: esapiext.TemplateMappingsSpec{
				Properties: map[string]esapiext.PropertySettings{
					"timestamp": {
						Type: "date",
					},
				},
			},
		},
	}
	drainStatusTemplate = esapiext.IndexTemplateSpec{
		TemplateName: drainStatusIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", drainStatusIndexPrefix),
		},
		Template: esapiext.TemplateSpec{
			Settings: esapiext.TemplateSettingsSpec{
				NumberOfShards:   2,
				NumberOfReplicas: 1,
				ISMPolicyID:      drainStatusPolicyName,
				RolloverAlias:    drainStatusIndexAlias,
			},
			Mappings: esapiext.TemplateMappingsSpec{
				Properties: map[string]esapiext.PropertySettings{
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

	normalIntervalIndexSettings = map[string]esapiext.TemplateMappingsSpec{
		"mappings": {
			Properties: map[string]esapiext.PropertySettings{
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

	kibanaDoc = esapiext.KibanaVersionDoc{
		DashboardVersion: kibanaDashboardVersion,
	}

	// kibanaObjects contains the ndjson form data for creating the kibana
	// index patterns and dashboards
	//go:embed dashboard.ndjson
	kibanaObjects string
)

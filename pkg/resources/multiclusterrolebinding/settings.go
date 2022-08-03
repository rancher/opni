package multiclusterrolebinding

import (
	"fmt"

	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/utils/pointer"
)

const (
	tracingPolicyName      = "tracing-policy"
	spanIndexPrefix        = "otel-v1-apm-span"
	spanIndexAlias         = "otel-v1-apm-span"
	spanIndexTemplateName  = "span-mapping"
	serviceMapIndexName    = "otel-v1-apm-service-map"
	serviceMapTemplateName = "servicemap-mapping"
)

var (
	oldTracingIndexPrefixes = []string{}
	clusterTerm             = `{"term":{"cluster_id.keyword": "${attr.internal.cluster}"}}`

	clusterIndexRole = osapiext.RoleSpec{
		RoleName: "cluster_index",
		ClusterPermissions: []string{
			"cluster_composite_ops",
			"cluster_monitor",
		},
		IndexPermissions: []osapiext.IndexPermissionSpec{
			{
				IndexPatterns: []string{
					"logs*",
				},
				AllowedActions: []string{
					"index",
					"indices:admin/get",
				},
			},
		},
	}

	opniSpanTemplate = osapiext.IndexTemplateSpec{
		TemplateName: spanIndexTemplateName,
		IndexPatterns: []string{
			fmt.Sprintf("%s*", spanIndexPrefix),
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   1,
				NumberOfReplicas: 1,
				ISMPolicyID:      tracingPolicyName,
				RolloverAlias:    spanIndexAlias,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				DateDetection: pointer.BoolPtr(false),
				DynamicTemplates: []map[string]osapiext.DynamicTemplateSpec{
					{
						"resource_attributes_map": osapiext.DynamicTemplateSpec{
							Mapping: osapiext.PropertySettings{
								Type: "keyword",
							},
							PathMatch: "resource.attributes.*",
						},
					},
					{
						"span_attributes_map": osapiext.DynamicTemplateSpec{
							Mapping: osapiext.PropertySettings{
								Type: "keyword",
							},
							PathMatch: "span.attributes.*",
						},
					},
				},
				Properties: map[string]osapiext.PropertySettings{
					"cluster_id": {
						Type: "keyword",
					},
					"traceId": {
						IgnoreAbove: 256,
						Type:        "keyword",
					},
					"spanId": {
						IgnoreAbove: 256,
						Type:        "keyword",
					},
					"parentSpanId": {
						IgnoreAbove: 256,
						Type:        "keyword",
					},
					"name": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
					"traceGroup": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
					"traceGroupFields": {
						Properties: map[string]osapiext.PropertySettings{
							"endTime": {
								Type: "date_nanos",
							},
							"durationInNanos": {
								Type: "long",
							},
							"statusCode": {
								Type: "integer",
							},
						},
					},
					"kind": {
						IgnoreAbove: 128,
						Type:        "keyword",
					},
					"startTime": {
						Type: "date_nanos",
					},
					"endTime": {
						Type: "date_nanos",
					},
					"status": {
						Properties: map[string]osapiext.PropertySettings{
							"code": {
								Type: "integer",
							},
							"message": {
								Type: "keyword",
							},
						},
					},
					"serviceName": {
						Type: "keyword",
					},
					"durationInNanos": {
						Type: "long",
					},
					"events": {
						Type: "nested",
						Properties: map[string]osapiext.PropertySettings{
							"time": {
								Type: "date_nanos",
							},
						},
					},
					"links": {
						Type: "nested",
					},
				},
			},
		},
		Priority: 100,
	}

	opniServiceMapTemplate = osapiext.IndexTemplateSpec{
		TemplateName: serviceMapTemplateName,
		IndexPatterns: []string{
			serviceMapIndexName,
		},
		Template: osapiext.TemplateSpec{
			Settings: osapiext.TemplateSettingsSpec{
				NumberOfShards:   1,
				NumberOfReplicas: 1,
			},
			Mappings: osapiext.TemplateMappingsSpec{
				DateDetection: pointer.BoolPtr(false),
				DynamicTemplates: []map[string]osapiext.DynamicTemplateSpec{
					{
						"strings_as_keyword": {
							Mapping: osapiext.PropertySettings{
								IgnoreAbove: 1024,
								Type:        "keyword",
							},
							MatchMappingType: "string",
						},
					},
				},
				Properties: map[string]osapiext.PropertySettings{
					"cluster_id": {
						Type: "keyword",
					},
					"hashId": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
					"serviceName": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
					"kind": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
					"destination": {
						Properties: map[string]osapiext.PropertySettings{
							"domain": {
								IgnoreAbove: 1024,
								Type:        "keyword",
							},
							"resource": {
								IgnoreAbove: 1024,
								Type:        "keyword",
							},
						},
					},
					"target": {
						Properties: map[string]osapiext.PropertySettings{
							"domain": {
								IgnoreAbove: 1024,
								Type:        "keyword",
							},
							"resource": {
								IgnoreAbove: 1024,
								Type:        "keyword",
							},
						},
					},
					"traceGroupName": {
						IgnoreAbove: 1024,
						Type:        "keyword",
					},
				},
			},
		},
	}
)

func (r *Reconciler) logISMPolicy() osapiext.ISMPolicySpec {
	return osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   indices.LogPolicyName,
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
						Retry: &indices.DefaultRetry,
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
						Retry: &indices.DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &indices.DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &indices.DefaultRetry,
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
						Retry: &indices.DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: func() string {
								if r.multiClusterRoleBinding.Spec.OpensearchConfig != nil {
									return r.multiClusterRoleBinding.Spec.OpensearchConfig.IndexRetention
								}
								return "7d"
							}(),
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
						Retry: &indices.DefaultRetry,
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", indices.LogIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
}

func (r *Reconciler) traceISMPolicy() osapiext.ISMPolicySpec {
	return osapiext.ISMPolicySpec{
		ISMPolicyIDSpec: &osapiext.ISMPolicyIDSpec{
			PolicyID:   tracingPolicyName,
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
						Retry: &indices.DefaultRetry,
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
						Retry: &indices.DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							IndexPriority: &osapiext.IndexPriorityOperation{
								Priority: 50,
							},
						},
						Retry: &indices.DefaultRetry,
					},
					{
						ActionOperation: &osapiext.ActionOperation{
							ForceMerge: &osapiext.ForceMergeOperation{
								MaxNumSegments: 1,
							},
						},
						Retry: &indices.DefaultRetry,
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
						Retry: &indices.DefaultRetry,
					},
				},
				Transitions: []osapiext.TransitionSpec{
					{
						StateName: "delete",
						Conditions: &osapiext.ConditionSpec{
							MinIndexAge: func() string {
								if r.multiClusterRoleBinding.Spec.OpensearchConfig != nil {
									return r.multiClusterRoleBinding.Spec.OpensearchConfig.IndexRetention
								}
								return "7d"
							}(),
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
						Retry: &indices.DefaultRetry,
					},
				},
				Transitions: make([]osapiext.TransitionSpec, 0),
			},
		},
		ISMTemplate: []osapiext.ISMTemplateSpec{
			{
				IndexPatterns: []string{
					fmt.Sprintf("%s*", spanIndexPrefix),
				},
				Priority: 100,
			},
		},
	}
}

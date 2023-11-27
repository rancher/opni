package kubernetes_manager

import (
	"encoding/base64"
	"encoding/json"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/encoding/protojson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	opniLabelKey           = "opni.io/managed"
	labelMatcherAnnotation = "opni.io/label_matcher"
	namespaceField         = "namespace"
	clusterField           = "cluster_id"
)

type DocumentQuery struct {
	Bool BoolQuery `json:"bool"`
}

type BoolQuery struct {
	Should             []Query `json:"should,omitempty"`
	Must               []Query `json:"must,omitempty"`
	MinimumShouldMatch *int    `json:"minimum_should_match,omitempty"`
}

type Query struct {
	Bool  *BoolQuery `json:"bool,omitempty"`
	Term  TermQuery  `json:"term,omitempty"`
	Terms TermsQuery `json:"terms,omitempty"`
}

type TermQuery map[string]TermValue
type TermsQuery map[string][]string

type TermValue struct {
	Value string `json:"value"`
}

func roleToOpensearch(namespace string, in *corev1.Role) *opsterv1.OpensearchRole {
	var matcherString string
	queryObj := DocumentQuery{
		Bool: BoolQuery{
			Must: []Query{},
		},
	}

	for _, permission := range in.GetPermissions() {
		var query Query
		switch corev1.CorePermissionType(permission.GetType()) {
		case corev1.PermissionTypeCluster:
			if permission.GetMatchLabels() != nil {
				matcherString = mustMarshalMatcherData(permission.GetMatchLabels())
			}

			switch l := len(permission.GetIds()); {
			case l == 1:
				query = Query{
					Term: TermQuery{
						clusterField: TermValue{
							Value: permission.GetIds()[0],
						},
					},
				}
			case l > 1:
				query = Query{
					Terms: TermsQuery{
						clusterField: permission.GetIds(),
					},
				}
			}
		case corev1.PermissionTypeNamespace:
			switch l := len(permission.GetIds()); {
			case l == 1:
				query = Query{
					Term: TermQuery{
						namespaceField: TermValue{
							Value: permission.GetIds()[0],
						},
					},
				}
			case l > 1:
				query = Query{
					Terms: TermsQuery{
						namespaceField: permission.GetIds(),
					},
				}
			}
		default:
			continue
		}
		queryObj.Bool.Must = append(queryObj.Bool.Must, query)
	}

	data, err := json.Marshal(queryObj)
	if err != nil {
		panic(err)
	}

	return &opsterv1.OpensearchRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.GetId(),
			Namespace: namespace,
			Annotations: func() map[string]string {
				if matcherString != "" {
					return map[string]string{
						labelMatcherAnnotation: matcherString,
					}
				}
				return nil
			}(),
			Labels: map[string]string{
				opniLabelKey: "true",
			},
		},
		Spec: opsterv1.OpensearchRoleSpec{
			IndexPermissions: []opsterv1.IndexPermissionSpec{
				{
					DocumentLevelSecurity: string(data),
					IndexPatterns: []string{
						"logs*",
					},
				},
			},
		},
	}
}

func opensearchToRole(in *opsterv1.OpensearchRole) *corev1.Role {
	queryString := in.Spec.IndexPermissions[0].DocumentLevelSecurity
	if queryString == "" {
		return &corev1.Role{
			Id: in.Name,
			Permissions: []*corev1.PermissionItem{
				{
					Verbs: []*corev1.PermissionVerb{
						corev1.VerbGet(),
					},
					Type: string(corev1.PermissionTypeCluster),
				},
				{
					Verbs: []*corev1.PermissionVerb{
						corev1.VerbGet(),
					},
					Type: string(corev1.PermissionTypeNamespace),
				},
			},
		}
	}

	query := DocumentQuery{}
	err := json.Unmarshal([]byte(queryString), &query)
	if err != nil {
		panic(err)
	}

	if len(query.Bool.Must) < 1 {
		// Invalid role query
		return nil
	}

	clusters := []string{}
	namespaces := []string{}
	for _, matcher := range query.Bool.Must {
		if term, ok := matcher.Term[clusterField]; ok {
			clusters = append(clusters, term.Value)
		}
		if terms, ok := matcher.Terms[clusterField]; ok {
			clusters = append(clusters, terms...)
		}
		if term, ok := matcher.Term[namespaceField]; ok {
			namespaces = append(namespaces, term.Value)
		}
		if terms, ok := matcher.Terms[namespaceField]; ok {
			namespaces = append(namespaces, terms...)
		}
	}

	return &corev1.Role{
		Id: in.Name,
		Permissions: []*corev1.PermissionItem{
			func() *corev1.PermissionItem {
				perm := &corev1.PermissionItem{
					Type: string(corev1.PermissionTypeCluster),
					Verbs: []*corev1.PermissionVerb{
						corev1.VerbGet(),
					},
				}
				if data, ok := in.Annotations[labelMatcherAnnotation]; ok {
					perm.MatchLabels = mustUnmarshalMatcherData(data)
					return perm
				}
				perm.Ids = clusters
				return perm
			}(),
			{
				Type: string(corev1.PermissionTypeNamespace),
				Verbs: []*corev1.PermissionVerb{
					corev1.VerbGet(),
				},
				Ids: namespaces,
			},
		},
		Metadata: &corev1.RoleMetadata{
			ResourceVersion: in.ResourceVersion,
		},
	}
}

func mustUnmarshalMatcherData(in string) *corev1.LabelSelector {
	data, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	selector := &corev1.LabelSelector{}
	err = protojson.Unmarshal(data, selector)
	if err != nil {
		panic(err)
	}
	return selector
}

func mustMarshalMatcherData(in *corev1.LabelSelector) string {
	data, err := protojson.Marshal(in)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

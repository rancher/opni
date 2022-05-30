package multiclusterrolebinding

import (
	"fmt"

	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	clusterTerm = `{"term":{"cluster_id.keyword": "${attr.internal.cluster}"}}`

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
)

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	username, password, retErr := helpers.UsernameAndPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	reconciler := opensearch.NewReconciler(
		r.ctx,
		opensearchCluster.Namespace,
		username,
		password,
		opensearchCluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	retErr = reconciler.MaybeCreateRole(clusterIndexRole)
	if retErr != nil {
		return
	}

	retErr = reconciler.ReconcileISM(r.ismPolicy())
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeCreateIndexTemplate(indices.OpniLogTemplate)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeBootstrapIndex(indices.LogIndexPrefix, indices.LogIndexAlias, indices.OldIndexPrefixes)
	if retErr != nil {
		return
	}

	return
}

func (r *Reconciler) deleteOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	username, password, err := helpers.UsernameAndPassword(r.ctx, r.client, cluster)
	if err != nil {
		return err
	}

	osReconciler := opensearch.NewReconciler(
		r.ctx,
		cluster.Namespace,
		username,
		password,
		cluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	err = osReconciler.MaybeDeleteRole(clusterIndexRole.RoleName)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiClusterRoleBinding), r.multiClusterRoleBinding); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.multiClusterRoleBinding, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.multiClusterRoleBinding)
	})
}

func (r *Reconciler) ismPolicy() osapiext.ISMPolicySpec {
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

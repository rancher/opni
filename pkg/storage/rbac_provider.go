package storage

import (
	"context"
	"fmt"
	"slices"
	"sort"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rbac"
	"go.uber.org/zap"
)

type rbacProvider struct {
	store  SubjectAccessCapableStore
	logger *zap.SugaredLogger
}

func NewRBACProvider(store SubjectAccessCapableStore) rbac.Provider {
	return &rbacProvider{
		store:  store,
		logger: logger.New().Named("rbac"),
	}
}

func (p *rbacProvider) SubjectAccess(
	ctx context.Context,
	req *corev1.SubjectAccessRequest,
) (*corev1.ReferenceList, error) {
	// Look up all role bindings which exist for this user, then look up the roles
	// referenced by those role bindings. Aggregate the resulting tenant IDs from
	// the roles and filter out any duplicates.
	rbs, err := p.store.ListRoleBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	allowedClusters := map[string]struct{}{}
	// All applicable role bindings for this user are ORed together
	for _, roleBinding := range rbs.Items {
		appliesToUser := roleBinding.Subject == req.Subject
		if !appliesToUser {
			continue
		}
		if taints := roleBinding.Taints; len(taints) > 0 {
			p.logger.With(
				"roleBinding", roleBinding.Id,
				"role", roleBinding.Id,
				"taints", roleBinding.Taints,
			).Warn("skipping tainted role binding")
			continue
		}
		for _, roleId := range roleBinding.GetRoleIds() {
			role, err := p.store.GetRole(ctx, &corev1.Reference{
				Id: roleId,
			})
			if err != nil {
				p.logger.With(
					zap.Error(err),
					"roleBinding", roleBinding.Id,
					"role", roleId,
				).Warn("error looking up role")
				continue
			}
			for _, permission := range role.Permissions {
				if permission.Type == string(corev1.PermissionTypeCluster) && slices.Contains(
					permission.GetVerbs(),
					&corev1.PermissionVerb{
						Verb: string(ClusterVerbGet),
					},
				) {
					// Add explicitly-allowed clusters to the list
					for _, clusterID := range permission.GetIds() {
						allowedClusters[clusterID] = struct{}{}
					}
					// Add any clusters to the list which match the role's label selector
					filteredList, err := p.store.ListClusters(ctx, permission.MatchLabels,
						corev1.MatchOptions_EmptySelectorMatchesNone)
					if err != nil {
						return nil, fmt.Errorf("failed to list clusters: %w", err)
					}
					for _, cluster := range filteredList.Items {
						allowedClusters[cluster.Id] = struct{}{}
					}
				}
			}
		}
	}
	sortedReferences := make([]*corev1.Reference, 0, len(allowedClusters))
	for clusterID := range allowedClusters {
		sortedReferences = append(sortedReferences, &corev1.Reference{
			Id: clusterID,
		})
	}
	sort.Slice(sortedReferences, func(i, j int) bool {
		return sortedReferences[i].Id < sortedReferences[j].Id
	})
	return &corev1.ReferenceList{
		Items: sortedReferences,
	}, nil
}

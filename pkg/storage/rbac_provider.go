package storage

import (
	"context"
	"fmt"
	"sort"

	"log/slog"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rbac"
)

type rbacProvider struct {
	store  SubjectAccessCapableStore
	logger *slog.Logger
}

func NewRBACProvider(store SubjectAccessCapableStore) rbac.Provider {
	return &rbacProvider{
		store:  store,
		logger: logger.New().WithGroup("rbac"),
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
		appliesToUser := false
		for _, s := range roleBinding.Subjects {
			if s == req.Subject {
				appliesToUser = true
			}
		}
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
		role, err := p.store.GetRole(ctx, roleBinding.RoleReference())
		if err != nil {
			p.logger.With(
				logger.Err(err),
				"roleBinding", roleBinding.Id,
				"role", roleBinding.RoleId,
			).Warn("error looking up role")
			continue
		}
		// Add explicitly-allowed clusters to the list
		for _, clusterID := range role.ClusterIDs {
			allowedClusters[clusterID] = struct{}{}
		}

		// Add any clusters to the list which match the role's label selector
		filteredList, err := p.store.ListClusters(ctx, role.MatchLabels,
			corev1.MatchOptions_EmptySelectorMatchesNone)
		if err != nil {
			return nil, fmt.Errorf("failed to list clusters: %w", err)
		}
		for _, cluster := range filteredList.Items {
			allowedClusters[cluster.Id] = struct{}{}
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

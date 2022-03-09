package storage

import (
	"context"
	"fmt"
	"sort"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"go.uber.org/zap"
)

type ClusterRBACStore interface {
	ClusterStore
	RBACStore
}

type rbacProvider struct {
	store  ClusterRBACStore
	logger *zap.SugaredLogger
}

func NewRBACProvider(store ClusterRBACStore) rbac.Provider {
	return &rbacProvider{
		store:  store,
		logger: logger.New().Named("rbac"),
	}
}

func (p *rbacProvider) SubjectAccess(
	ctx context.Context,
	req *core.SubjectAccessRequest,
) (*core.ReferenceList, error) {
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
				zap.Error(err),
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
			core.MatchOptions_EmptySelectorMatchesNone)
		if err != nil {
			return nil, fmt.Errorf("failed to list clusters: %w", err)
		}
		for _, cluster := range filteredList.Items {
			allowedClusters[cluster.Id] = struct{}{}
		}
	}
	sortedReferences := make([]*core.Reference, 0, len(allowedClusters))
	for clusterID := range allowedClusters {
		sortedReferences = append(sortedReferences, &core.Reference{
			Id: clusterID,
		})
	}
	sort.Slice(sortedReferences, func(i, j int) bool {
		return sortedReferences[i].Id < sortedReferences[j].Id
	})
	return &core.ReferenceList{
		Items: sortedReferences,
	}, nil
}

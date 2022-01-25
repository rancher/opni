package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/rbac"
	"go.uber.org/zap"
)

type rbacProvider struct {
	rbacStore RBACStore
	logger    *zap.SugaredLogger
}

func NewRBACProvider(rbacStore RBACStore) rbac.Provider {
	return &rbacProvider{
		rbacStore: rbacStore,
		logger:    logger.New().Named("rbac"),
	}
}

func (p *rbacProvider) ListTenantsForUser(ctx context.Context, userID string) ([]string, error) {
	// Look up all role bindings which exist for this user, then look up the roles
	// referenced by those role bindings. Aggregate the resulting tenant IDs from
	// the roles and filter out any duplicates.
	ctx, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	rbs, err := p.rbacStore.ListRoleBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	tenants := map[string]struct{}{}
	for _, roleBinding := range rbs {
		if roleBinding.UserID == userID {
			// Look up the associated role
			role, err := p.rbacStore.GetRole(ctx, roleBinding.RoleName)
			if err != nil {
				p.logger.With(
					zap.Error(err),
					zap.String("role", roleBinding.RoleName),
				).Warn("ignoring invalid role binding")
				continue
			}
			for _, tenantID := range role.TenantIDs {
				if strings.TrimSpace(tenantID) == "" {
					continue
				}
				tenants[tenantID] = struct{}{}
			}
		}
	}
	tenantIDs := make([]string, 0, len(tenants))
	for tenantID := range tenants {
		tenantIDs = append(tenantIDs, tenantID)
	}
	return tenantIDs, nil
}

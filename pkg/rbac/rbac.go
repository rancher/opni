package rbac

import (
	"context"
)

const (
	UserIDKey = "rbac_user_id"
)

type Role struct {
	Name      string   `json:"name"`
	TenantIDs []string `json:"tenantIDs"`
}

type RoleBinding struct {
	Name     string `json:"name"`
	RoleName string `json:"roleName"`
	UserID   string `json:"userID"`
}

type Provider interface {
	ListTenantsForUser(ctx context.Context, userID string) ([]string, error)
}

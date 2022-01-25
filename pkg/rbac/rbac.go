package rbac

import (
	"context"
)

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	ListTenantsForUser(ctx context.Context, userID string) ([]string, error)
}

package rbac

import (
	"context"

	"github.com/kralicky/opni-monitoring/pkg/core"
)

const (
	UserIDKey = "rbac_user_id"
)

type Provider interface {
	SubjectAccess(context.Context, *core.SubjectAccessRequest) (*core.ReferenceList, error)
}

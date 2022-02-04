package storage

import (
	"context"
	"errors"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

var ErrNotFound = errors.New("not found")

type TokenStore interface {
	CreateToken(ctx context.Context, ttl time.Duration) (*tokens.Token, error)
	DeleteToken(ctx context.Context, ref *core.Reference) error
	TokenExists(ctx context.Context, ref *core.Reference) (bool, error)
	GetToken(ctx context.Context, ref *core.Reference) (*tokens.Token, error)
	ListTokens(ctx context.Context) ([]*tokens.Token, error)
}

type ClusterStore interface {
	CreateCluster(ctx context.Context, cluster *core.Cluster) error
	DeleteCluster(ctx context.Context, ref *core.Reference) error
	ClusterExists(ctx context.Context, ref *core.Reference) (bool, error)
	GetCluster(ctx context.Context, ref *core.Reference) (*core.Cluster, error)
	UpdateCluster(ctx context.Context, cluster *core.Cluster) (*core.Cluster, error)
	ListClusters(ctx context.Context, matchLabels *core.LabelSelector, matchOptions core.MatchOptions) (*core.ClusterList, error)
	KeyringStore(ctx context.Context, ref *core.Reference) (KeyringStore, error)
}

type RBACStore interface {
	CreateRole(context.Context, *core.Role) error
	DeleteRole(context.Context, *core.Reference) error
	GetRole(context.Context, *core.Reference) (*core.Role, error)
	CreateRoleBinding(context.Context, *core.RoleBinding) error
	DeleteRoleBinding(context.Context, *core.Reference) error
	GetRoleBinding(context.Context, *core.Reference) (*core.RoleBinding, error)
	ListRoles(context.Context) (*core.RoleList, error)
	ListRoleBindings(context.Context) (*core.RoleBindingList, error)
}

type KeyringStore interface {
	Put(ctx context.Context, keyring keyring.Keyring) error
	Get(ctx context.Context) (keyring.Keyring, error)
}

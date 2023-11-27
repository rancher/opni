package backend

import (
	"context"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
)

type InstallState int

const (
	Pending InstallState = iota
	Installed
	Absent
	Error
)

type ClusterDriver interface {
	GetInstallStatus(context.Context) InstallState
	StoreCluster(context.Context, *corev1.Reference, string) error
	StoreClusterMetadata(context.Context, string, string) error
	StoreClusterReadUser(ctx context.Context, username, password, id string) error
	DeleteCluster(context.Context, string) error
	SetClusterStatus(context.Context, string, bool) error
	GetClusterStatus(context.Context, string) (*capabilityv1.NodeCapabilityStatus, error)

	SetSyncTime()
}

type RBACDriver interface {
	GetRole(context.Context, *corev1.Reference) (*corev1.Role, error)
	CreateRole(context.Context, *corev1.Role) error
	UpdateRole(context.Context, *corev1.Role) error
	DeleteRole(context.Context, *corev1.Reference) error
	ListRoles(context.Context) (*corev1.RoleList, error)
	GetBackendURL(context.Context) (string, error)
}

var (
	ClusterDrivers = driverutil.NewCache[ClusterDriver]()
	RBACDrivers    = driverutil.NewCache[RBACDriver]()
)

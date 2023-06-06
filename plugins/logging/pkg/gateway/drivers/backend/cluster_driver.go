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
	StoreCluster(context.Context, *corev1.Reference) error
	StoreClusterMetadata(context.Context, string, string) error
	DeleteCluster(context.Context, string) error
	SetClusterStatus(context.Context, string, bool) error
	GetClusterStatus(context.Context, string) (*capabilityv1.NodeCapabilityStatus, error)
	SetSyncTime()
}

var Drivers = driverutil.NewDriverCache[ClusterDriver]()

package management

import (
	"context"

	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	corev1 "k8s.io/api/core/v1"
)

type ClusterDriver interface {
	Name() string
	AdminPassword(context.Context) ([]byte, error)
	NewOpensearchClientForCluster(context.Context) *opensearch.Client
	GetCluster(context.Context) (*loggingadmin.OpensearchClusterV2, error)
	DeleteCluster(context.Context) error
	CreateOrUpdateCluster(context.Context, *loggingadmin.OpensearchClusterV2, string, *corev1.LocalObjectReference) error
	UpgradeAvailable(context.Context, string) (bool, error)
	DoUpgrade(context.Context, string) error
	GetStorageClasses(context.Context) ([]string, error)
}

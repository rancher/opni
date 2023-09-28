package backend

import (
	"context"
	"fmt"
	"sort"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/metrics/pkg/constants"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RBACBackend struct {
	capabilityv1.UnsafeRBACManagerServer
	RBACBackendConfig
	util.Initializer
}

type RBACBackendConfig struct {
	Logger         *zap.SugaredLogger `validate:"required"`
	RoleStore      storage.RoleStore
	StorageBackend storage.Backend
}

func (r *RBACBackend) Initialize(conf RBACBackendConfig) {
	r.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		r.RBACBackendConfig = conf
	})
}

func (r *RBACBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	// Info must not block
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityMetrics,
		Source:  "plugin_metrics",
		Drivers: drivers.ClusterDrivers.List(),
	}, nil
}

func (r *RBACBackend) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*v1.AvailablePermissions, error) {
	return &v1.AvailablePermissions{
		Items: []*v1.PermissionDescription{
			{
				Type: string(v1.PermissionTypeCluster),
				Verbs: []*v1.PermissionVerb{
					v1.VerbGet(),
				},
			},
		},
	}, nil
}

func (r *RBACBackend) GetRole(ctx context.Context, in *v1.Reference) (*v1.Role, error) {
	r.WaitForInit()
	role, err := r.RoleStore.Get(ctx, in.GetId())
	if err != nil {
		return nil, err
	}
	return role, nil
}

func (r *RBACBackend) CreateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	r.WaitForInit()

	_, err := r.RoleStore.Get(ctx, in.Reference().GetId())
	if err == nil {
		return nil, storage.ErrAlreadyExists
	}
	if !storage.IsNotFound(err) {
		return nil, err
	}

	err = r.RoleStore.Put(ctx, in.GetId(), in)
	return &emptypb.Empty{}, err
}

func (r *RBACBackend) UpdateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	r.WaitForInit()

	oldRole, err := r.RoleStore.Get(ctx, in.Reference().GetId())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	oldRole.Permissions = in.GetPermissions()
	oldRole.Metadata = in.GetMetadata()
	err = r.RoleStore.Put(ctx, oldRole.Reference().GetId(), oldRole)
	return &emptypb.Empty{}, err
}

func (r *RBACBackend) DeleteRole(ctx context.Context, in *v1.Reference) (*emptypb.Empty, error) {
	r.WaitForInit()
	err := r.RoleStore.Delete(ctx, in.GetId())
	return &emptypb.Empty{}, err
}

func (r *RBACBackend) ListRoles(ctx context.Context, _ *emptypb.Empty) (*v1.RoleList, error) {
	r.WaitForInit()
	keys, err := r.RoleStore.ListKeys(ctx, "")
	if err != nil {
		return nil, err
	}

	roles := []*v1.Reference{}
	for _, key := range keys {
		roles = append(roles, &v1.Reference{
			Id: key,
		})
	}
	return &v1.RoleList{
		Items: roles,
	}, nil
}

func (r *RBACBackend) AccessHeader(ctx context.Context, roles *v1.ReferenceList) (rbac.RBACHeader, error) {
	allowedClusters := map[string]struct{}{}
	for _, role := range roles.GetItems() {
		role, err := r.RoleStore.Get(ctx, role.GetId())
		if err != nil {
			r.Logger.With(
				zap.Error(err),
				"role", role.GetId(),
			).Warn("error looking up role")
			continue
		}
		for _, permission := range role.Permissions {
			if permission.Type == string(v1.PermissionTypeCluster) && v1.VerbGet().InList(permission.GetVerbs()) {
				// Add explicitly-allowed clusters to the list
				for _, clusterID := range permission.GetIds() {
					allowedClusters[clusterID] = struct{}{}
				}
				// Add any clusters to the list which match the role's label selector
				filteredList, err := r.StorageBackend.ListClusters(ctx, permission.MatchLabels,
					v1.MatchOptions_EmptySelectorMatchesNone)
				if err != nil {
					return nil, fmt.Errorf("failed to list clusters: %w", err)
				}
				for _, cluster := range filteredList.Items {
					allowedClusters[cluster.Id] = struct{}{}
				}
			}
		}
	}

	sortedReferences := make([]*v1.Reference, 0, len(allowedClusters))
	for clusterID := range allowedClusters {
		sortedReferences = append(sortedReferences, &v1.Reference{
			Id: clusterID,
		})
	}
	sort.Slice(sortedReferences, func(i, j int) bool {
		return sortedReferences[i].Id < sortedReferences[j].Id
	})
	return rbac.RBACHeader{
		constants.AuthorizedClusterIDsKey: &v1.ReferenceList{
			Items: sortedReferences,
		},
	}, nil
}

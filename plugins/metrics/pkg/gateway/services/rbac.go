package services

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/metrics/pkg/constants"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const rolePrefix = "role_"

type RBACBackendService struct {
	Context types.ServiceContext `option:"context"`

	rolesStore storage.RoleStore
}

var _ types.Service = (*RBACBackendService)(nil)

func (r *RBACBackendService) Activate() error {
	r.rolesStore = newRolesStore(r.Context.KeyValueStoreClient())
	return nil
}

func (r *RBACBackendService) AddToScheme(scheme meta.Scheme) {
	scheme.Add(capability.CapabilityRBACPluginID, capability.NewRBACPlugin(r))
}

func (r *RBACBackendService) info() *capabilityv1.Details {
	return &capabilityv1.Details{
		Name:             wellknown.CapabilityMetrics,
		Source:           "plugin_metrics",
		AvailableDrivers: drivers.ClusterDrivers.List(),
		EnabledDriver:    r.Context.GatewayConfig().Spec.Cortex.Management.ClusterDriver,
	}
}

func (r *RBACBackendService) Info(_ context.Context, capability *corev1.Reference) (*capabilityv1.Details, error) {
	// Info must not block
	if capability.GetId() != wellknown.CapabilityMetrics {
		return nil, status.Errorf(codes.InvalidArgument, "capability %s not implemented by this plugin", capability.GetId())
	}
	return r.info(), nil
}

func (r *RBACBackendService) List(_ context.Context, _ *emptypb.Empty) (*capabilityv1.DetailsList, error) {
	return &capabilityv1.DetailsList{
		Items: []*capabilityv1.Details{
			r.info(),
		},
	}, nil
}

func (r *RBACBackendService) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*corev1.AvailablePermissions, error) {
	return &corev1.AvailablePermissions{
		Items: []*corev1.PermissionDescription{
			{
				Type: string(corev1.PermissionTypeCluster),
				Verbs: []*corev1.PermissionVerb{
					corev1.VerbGet(),
				},
			},
		},
	}, nil
}

func (r *RBACBackendService) GetRole(ctx context.Context, in *corev1.Reference) (*corev1.Role, error) {
	var revision int64
	role, err := r.rolesStore.Get(ctx, in.GetId(), storage.WithRevisionOut(&revision))
	if err != nil {
		return nil, err
	}
	role.Metadata = &corev1.RoleMetadata{
		ResourceVersion: strconv.FormatInt(revision, 10),
	}
	return role, nil
}

func (r *RBACBackendService) CreateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	_, err := r.rolesStore.Get(ctx, in.Reference().GetId())
	if err == nil {
		return nil, storage.ErrAlreadyExists
	}
	if !storage.IsNotFound(err) {
		return nil, err
	}

	locker := system.NewLock(ctx, r.Context.KeyValueStoreClient(), lockKey(in.Reference().GetId()))

	var innerErr error
	err = locker.Try(func() {
		innerErr = r.rolesStore.Put(ctx, in.GetId(), in)
	}, func() {
		innerErr = status.Error(codes.Aborted, "could not obtain lock")
	})

	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, innerErr
}

func (r *RBACBackendService) UpdateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	locker := system.NewLock(ctx, r.Context.KeyValueStoreClient(), lockKey(in.Reference().GetId()))
	var innerErr error

	err := locker.Do(func() {
		oldRole, err := r.rolesStore.Get(ctx, in.Reference().GetId())
		if err != nil {
			innerErr = err
			return
		}
		revision, err := strconv.ParseInt(in.GetMetadata().GetResourceVersion(), 10, 64)
		if err != nil {
			innerErr = err
			return
		}
		oldRole.Permissions = in.GetPermissions()
		innerErr = r.rolesStore.Put(ctx, oldRole.Reference().GetId(), oldRole, storage.WithRevision(revision))
	})

	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, innerErr
}

func (r *RBACBackendService) DeleteRole(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	locker := system.NewLock(ctx, r.Context.KeyValueStoreClient(), lockKey(in.GetId()))
	var innerErr error
	err := locker.Do(func() {
		innerErr = r.rolesStore.Delete(ctx, in.GetId())
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, innerErr
}

func (r *RBACBackendService) ListRoles(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleList, error) {
	keys, err := r.rolesStore.ListKeys(ctx, "")
	if err != nil {
		return nil, err
	}

	roles := []*corev1.Reference{}
	for _, key := range keys {
		roles = append(roles, &corev1.Reference{
			Id: key,
		})
	}
	return &corev1.RoleList{
		Items: roles,
	}, nil
}

func lockKey(key string) string {
	return rolePrefix + key
}

func newRolesStore(client system.KeyValueStoreClient) storage.RoleStore {
	return kvutil.WithPrefix(system.NewKVStoreClient[*corev1.Role](client), "/roles")
}

type RBACProvider struct {
	context types.ServiceContext
}

func NewRBACProvider(context types.ServiceContext) *RBACProvider {
	return &RBACProvider{
		context: context,
	}
}

func (r *RBACProvider) AccessHeader(ctx context.Context, roles *corev1.ReferenceList) (rbac.RBACHeader, error) {
	rolesStore := newRolesStore(r.context.KeyValueStoreClient())
	allowedClusters := map[string]struct{}{}
	for _, role := range roles.GetItems() {
		role, err := rolesStore.Get(ctx, role.GetId())
		if err != nil {
			r.context.Logger().With(
				zap.Error(err),
				"role", role.GetId(),
			).Warn("error looking up role")
			continue
		}
		for _, permission := range role.Permissions {
			if permission.Type == string(corev1.PermissionTypeCluster) && corev1.VerbGet().InList(permission.GetVerbs()) {
				// Add explicitly-allowed clusters to the list
				for _, clusterID := range permission.GetIds() {
					allowedClusters[clusterID] = struct{}{}
				}
				// Add any clusters to the list which match the role's label selector
				filteredList, err := r.context.StorageBackend().ListClusters(ctx, permission.MatchLabels,
					corev1.MatchOptions_EmptySelectorMatchesNone)
				if err != nil {
					return nil, fmt.Errorf("failed to list clusters: %w", err)
				}
				for _, cluster := range filteredList.Items {
					allowedClusters[cluster.Id] = struct{}{}
				}
			}
		}
	}

	sortedReferences := make([]*corev1.Reference, 0, len(allowedClusters))
	for clusterID := range allowedClusters {
		sortedReferences = append(sortedReferences, &corev1.Reference{
			Id: clusterID,
		})
	}
	sort.Slice(sortedReferences, func(i, j int) bool {
		return sortedReferences[i].Id < sortedReferences[j].Id
	})
	return rbac.RBACHeader{
		constants.AuthorizedClusterIDsKey: &corev1.ReferenceList{
			Items: sortedReferences,
		},
	}, nil
}

func init() {
	types.Services.Register("Capability RBAC Backend Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &RBACBackendService{}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}

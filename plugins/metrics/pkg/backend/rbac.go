package backend

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	AuthorizedClusterIDsKey = "authorized_cluster_ids"
)

func (m *MetricsBackend) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*v1.AvailablePermissions, error) {
	return &v1.AvailablePermissions{
		Items: []*v1.PermissionDescription{
			{
				Type: string(v1.PermissionTypeCluster),
				Verbs: []*v1.PermissionVerb{
					{Verb: string(storage.ClusterVerbGet)},
				},
			},
		},
	}, nil
}

func (m *MetricsBackend) GetRole(ctx context.Context, in *v1.Reference) (*v1.Role, error) {
	m.WaitForInit()
	return m.RolesStore.GetRole(ctx, in)
}

func (m *MetricsBackend) CreateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	m.WaitForInit()
	err := m.RolesStore.CreateRole(ctx, in)
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) UpdateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	m.WaitForInit()

	oldRole, err := m.RolesStore.GetRole(ctx, in.Reference())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	_, err = m.RolesStore.UpdateRole(ctx, oldRole.Reference(), func(role *v1.Role) {
		role.Permissions = in.GetPermissions()
		role.Metadata = in.GetMetadata()
	})
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) DeleteRole(ctx context.Context, in *v1.Reference) (*emptypb.Empty, error) {
	m.WaitForInit()
	err := m.RolesStore.DeleteRole(ctx, in)
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) ListRoles(ctx context.Context, _ *emptypb.Empty) (*v1.RoleList, error) {
	m.WaitForInit()
	return m.RolesStore.ListRoles(ctx)
}

func (m *MetricsBackend) AccessHeader(ctx context.Context, roles *v1.ReferenceList) (rbac.RBACHeader, error) {
	allowedClusters := map[string]struct{}{}
	for _, role := range roles.GetItems() {
		role, err := m.RolesStore.GetRole(ctx, role)
		if err != nil {
			m.Logger.With(
				zap.Error(err),
				"role", role.GetId(),
			).Warn("error looking up role")
			continue
		}
		for _, permission := range role.Permissions {
			if permission.Type == string(v1.PermissionTypeCluster) && slices.Contains(
				permission.GetVerbs(),
				&v1.PermissionVerb{
					Verb: string(storage.ClusterVerbGet),
				},
			) {
				// Add explicitly-allowed clusters to the list
				for _, clusterID := range permission.GetIds() {
					allowedClusters[clusterID] = struct{}{}
				}
				// Add any clusters to the list which match the role's label selector
				filteredList, err := m.StorageBackend.ListClusters(ctx, permission.MatchLabels,
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
		AuthorizedClusterIDsKey: &v1.ReferenceList{
			Items: sortedReferences,
		},
	}, nil
}

func AuthorizedClusterIDs(c *gin.Context) []string {
	value, ok := c.Get(AuthorizedClusterIDsKey)
	if !ok {
		return nil
	}
	return value.([]string)
}

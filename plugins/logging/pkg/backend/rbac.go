package backend

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	utilerrors "github.com/rancher/opni/pkg/util/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (b *LoggingBackend) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*corev1.AvailablePermissions, error) {
	return &corev1.AvailablePermissions{
		Items: []*corev1.PermissionDescription{
			{
				Type: string(corev1.PermissionTypeCluster),
				Verbs: []*corev1.PermissionVerb{
					corev1.VerbGet(),
				},
				EnableLabelMatch: true,
			},
			{
				Type: string(corev1.PermissionTypeNamespace),
				Verbs: []*corev1.PermissionVerb{
					corev1.VerbGet(),
				},
				EnableLabelMatch: false,
			},
		},
	}, nil
}

func (b *LoggingBackend) GetRole(ctx context.Context, ref *corev1.Reference) (*corev1.Role, error) {
	role, err := b.RBACDriver.GetRole(ctx, ref)
	if err != nil {
		return nil, err
	}
	for _, permission := range role.GetPermissions() {
		cleanClusterPermission(permission)
	}
	return role, nil
}

func (b *LoggingBackend) CreateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	for _, permission := range in.GetPermissions() {
		err := b.updateClusterPermissionIDs(ctx, permission)
		if err != nil {
			return nil, utilerrors.New(codes.Aborted, err)
		}
	}

	err := b.RBACDriver.CreateRole(ctx, in)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (b *LoggingBackend) UpdateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	for _, permission := range in.GetPermissions() {
		err := b.updateClusterPermissionIDs(ctx, permission)
		if err != nil {
			return nil, utilerrors.New(codes.Aborted, err)
		}
	}
	err := b.RBACDriver.UpdateRole(ctx, in)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (b *LoggingBackend) DeleteRole(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	err := b.RBACDriver.DeleteRole(ctx, in)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (b *LoggingBackend) ListRoles(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleList, error) {
	list, err := b.RBACDriver.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (b *LoggingBackend) clusterIDsFromMatcher(ctx context.Context, ml *corev1.LabelSelector) ([]string, error) {
	filteredList, err := b.StorageBackend.ListClusters(ctx, ml,
		corev1.MatchOptions_EmptySelectorMatchesNone)
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	clusters := make([]string, len(filteredList.GetItems()))
	for i, cluster := range filteredList.GetItems() {
		clusters[i] = cluster.GetId()
	}
	return clusters, nil
}

func (b *LoggingBackend) updateClusterPermissionIDs(ctx context.Context, in *corev1.PermissionItem) error {
	if in.GetType() != string(corev1.PermissionTypeCluster) {
		return nil
	}

	if in.GetMatchLabels() == nil {
		return nil
	}
	ids, err := b.clusterIDsFromMatcher(ctx, in.GetMatchLabels())
	if err != nil {
		return err
	}
	in.Ids = ids
	return nil
}

// When ever the clusters get updated recalculate the matchlabels
func (b *LoggingBackend) updateRoles(ctx context.Context, event *managementv1.WatchEvent) error {
	list, err := b.ListRoles(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}

	for _, ref := range list.GetItems() {
		role, err := b.GetRole(ctx, ref)
		if err != nil {
			return err
		}
		_, err = b.UpdateRole(ctx, role)
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanClusterPermission(in *corev1.PermissionItem) {
	if in.GetType() != string(corev1.PermissionTypeCluster) {
		return
	}
	if in.GetMatchLabels() != nil {
		in.Ids = nil
	}
}

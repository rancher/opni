package machinery

import (
	"context"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/types/known/emptypb"
)

type subjectAccessCapableStore struct {
	client management.ManagementClient
}

func SubjectAccessCapableStore(client management.ManagementClient) storage.SubjectAccessCapableStore {
	return &subjectAccessCapableStore{
		client: client,
	}
}

func (s *subjectAccessCapableStore) ListClusters(
	ctx context.Context,
	matchLabels *core.LabelSelector,
	matchOptions core.MatchOptions,
) (*core.ClusterList, error) {
	return s.client.ListClusters(ctx, &management.ListClustersRequest{
		MatchLabels:  matchLabels,
		MatchOptions: matchOptions,
	})
}

func (s *subjectAccessCapableStore) GetRole(
	ctx context.Context,
	ref *core.Reference,
) (*core.Role, error) {
	return s.client.GetRole(ctx, ref)
}

func (s *subjectAccessCapableStore) ListRoleBindings(
	ctx context.Context,
) (*core.RoleBindingList, error) {
	return s.client.ListRoleBindings(ctx, &emptypb.Empty{})
}

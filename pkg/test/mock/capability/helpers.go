package mock_v1

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni/pkg/apis/capability/v1"
	v12 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type CapabilityInfo struct {
	Name              string
	CanInstall        bool
	InstallerTemplate string
	Storage           storage.ClusterStore
}

func (ci *CapabilityInfo) canInstall() error {
	if !ci.CanInstall {
		return errors.New("test error")
	}
	return nil
}

func NewTestCapabilityBackend(
	ctrl *gomock.Controller,
	capBackend *CapabilityInfo,
) v1.BackendClient {
	client := NewMockBackendClient(ctrl)
	client.EXPECT().
		Info(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&v1.Details{
			Name:    capBackend.Name,
			Source:  "mock",
			Drivers: []string{"test"},
		}, nil).
		AnyTimes()
	client.EXPECT().
		CanInstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, capBackend.canInstall()
		}).
		AnyTimes()
	client.EXPECT().
		Install(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *v1.InstallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			_, err := capBackend.Storage.UpdateCluster(ctx, req.Cluster,
				storage.NewAddCapabilityMutator[*v12.Cluster](capabilities.Cluster("test")),
			)
			if err != nil {
				return nil, err
			}
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	client.EXPECT().
		Uninstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *v1.UninstallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			_, err := capBackend.Storage.UpdateCluster(ctx, req.Cluster,
				storage.NewRemoveCapabilityMutator[*v12.Cluster](capabilities.Cluster("test")))
			if err != nil {
				return nil, err
			}
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	client.EXPECT().
		UninstallStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *v12.Reference, _ ...grpc.CallOption) (*v12.TaskStatus, error) {
			c, err := capBackend.Storage.GetCluster(ctx, ref)
			if err != nil {
				return nil, err
			}
			for _, cap := range c.GetCapabilities() {
				if cap.Name == "test" {
					if cap.DeletionTimestamp != nil {
						return &v12.TaskStatus{
							State: v12.TaskState_Running,
						}, nil
					} else {
						return &v12.TaskStatus{
							State: v12.TaskState_Unknown,
						}, nil
					}
				}
			}
			return &v12.TaskStatus{
				State: v12.TaskState_Completed,
			}, nil
		}).
		AnyTimes()
	client.EXPECT().
		CancelUninstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *emptypb.Empty, ...grpc.CallOption) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	client.EXPECT().
		InstallerTemplate(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&v1.InstallerTemplateResponse{
			Template: capBackend.InstallerTemplate,
		}, nil).
		AnyTimes()
	return client
}

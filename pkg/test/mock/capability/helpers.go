package mock_v1

import (
	"context"
	"errors"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"go.uber.org/mock/gomock"
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
) capabilityv1.BackendClient {
	client := NewMockBackendClient(ctrl)
	client.EXPECT().
		Info(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&capabilityv1.Details{
			Name:             capBackend.Name,
			Source:           "mock",
			AvailableDrivers: []string{"test"},
			EnabledDriver:    "test",
		}, nil).
		AnyTimes()
	client.EXPECT().
		Install(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *capabilityv1.InstallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			_, err := capBackend.Storage.UpdateCluster(ctx, req.Agent,
				storage.NewAddCapabilityMutator[*corev1.Cluster](capabilities.Cluster("test")),
			)
			if err != nil {
				return nil, err
			}
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	client.EXPECT().
		Uninstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *capabilityv1.UninstallRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			_, err := capBackend.Storage.UpdateCluster(ctx, req.Agent,
				storage.NewRemoveCapabilityMutator[*corev1.Cluster](capabilities.Cluster("test")))
			if err != nil {
				return nil, err
			}
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	client.EXPECT().
		UninstallStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *capabilityv1.UninstallStatusRequest, _ ...grpc.CallOption) (*corev1.TaskStatus, error) {
			c, err := capBackend.Storage.GetCluster(ctx, req.Agent)
			if err != nil {
				return nil, err
			}
			for _, cap := range c.GetCapabilities() {
				if cap.Name == "test" {
					if cap.DeletionTimestamp != nil {
						return &corev1.TaskStatus{
							State: corev1.TaskState_Running,
						}, nil
					} else {
						return &corev1.TaskStatus{
							State: corev1.TaskState_Unknown,
						}, nil
					}
				}
			}
			return &corev1.TaskStatus{
				State: corev1.TaskState_Completed,
			}, nil
		}).
		AnyTimes()
	client.EXPECT().
		CancelUninstall(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *capabilityv1.CancelUninstallRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
			return &emptypb.Empty{}, nil
		}).
		AnyTimes()
	return client
}

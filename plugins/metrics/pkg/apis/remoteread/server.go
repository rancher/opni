package remoteread

import (
	"context"
	"fmt"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
)

// block compilation until type assertion is satisfied
var _ RemoteReadServer = (*RemoteRead)(nil)

type RemoteReadConfig struct {
	remoteWriterClient remotewrite.RemoteWriteClient `validate:"required"`
	Config             *v1beta1.GatewayConfig        `validate:"required"`
	Logger             *zap.SugaredLogger            `validate:"required"`
}

type RemoteRead struct {
	UnsafeRemoteReadServer
	RemoteReadConfig
	util.Initializer

	// todo: might need something more robust
	targets   map[string]*Target
	targetsMu sync.RWMutex
}

func (reader *RemoteRead) Initialize(conf RemoteReadConfig) {
	reader.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}

		reader.RemoteReadConfig = conf
	})
}

func (reader *RemoteRead) AddTarget(_ context.Context, request *TargetAddRequest) (*Target, error) {
	target := request.Target

	reader.targetsMu.Lock()
	defer reader.targetsMu.Unlock()

	reader.targets[target.Name] = target

	return target, nil
}

func (reader *RemoteRead) EditTarget(_ context.Context, request *TargetEditRequest) (*Target, error) {
	diff := request.TargetDiff

	reader.targetsMu.Lock()
	defer reader.targetsMu.Unlock()

	target := reader.targets[request.TargetName]

	if diff.Endpoint != "" {
		target.Endpoint = diff.Endpoint
	}

	if diff.Name != "" {
		target.Name = diff.Name

		delete(reader.targets, request.TargetName)
	}

	reader.targets[target.Name] = target

	return target, nil
}

func (reader *RemoteRead) RemoveTarget(_ context.Context, request *TargetRemoveRequest) (*Target, error) {
	reader.targetsMu.Lock()
	defer reader.targetsMu.Unlock()

	if target, found := reader.targets[request.TargetName]; found {
		delete(reader.targets, request.TargetName)
		return target, nil
	}

	return nil, fmt.Errorf("no target '%s' found", request.TargetName)
}

func (reader *RemoteRead) ListTargets(_ context.Context, _ *emptypb.Empty) (*TargetList, error) {
	reader.targetsMu.Lock()
	defer reader.targetsMu.Unlock()

	targets := make([]*Target, len(reader.targets))

	for _, target := range reader.targets {
		targets = append(targets, target)
	}

	return &TargetList{
		Targets: targets,
	}, nil
}

func (reader *RemoteRead) Start(_ context.Context, _ *StartReadRequest) (*emptypb.Empty, error) {
	// todo: implement
	return nil, fmt.Errorf("not yet implemented")
}

//func (reader *RemoteRead) GetProgress(_ context.Context, _ *ProgressRequest) (*emptypb.Empty, error) {
//	// todo: implement
//	return nil, fmt.Errorf("not yet implemented")
//}

func (reader *RemoteRead) Stop(_ context.Context, _ *StopReadRequest) (*emptypb.Empty, error) {
	// todo: implement
	return nil, fmt.Errorf("not yet implemented")
}

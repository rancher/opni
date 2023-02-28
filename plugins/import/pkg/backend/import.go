package backend

import (
	"context"
	"fmt"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"strings"
	"sync"
)

func targetAlreadyExistsError(id string) error {
	return status.Errorf(codes.AlreadyExists, "target '%s' already exists", id)
}

func targetDoesNotExistError(id string) error {
	return status.Errorf(codes.NotFound, "target '%s' not found", id)
}

func getIdFromTargetMeta(meta *remoteread.TargetMeta) string {
	return fmt.Sprintf("%s/%s", meta.ClusterId, meta.Name)
}

type ImportBackendConfig struct {
	Logger   *zap.SugaredLogger                                         `validate:"required"`
	Delegate streamext.StreamDelegate[remoteread.RemoteReadAgentClient] `validate:"required"`
}

var _ remoteread.RemoteReadGatewayServer = (*ImportBackend)(nil)

type ImportBackend struct {
	remoteread.UnsafeRemoteReadGatewayServer
	util.Initializer
	ImportBackendConfig

	// the stored remoteread.Target should never have their status populated
	remoteReadTargetMu sync.RWMutex
	remoteReadTargets  map[string]*remoteread.Target
}

func (b *ImportBackend) Initialize(conf ImportBackendConfig) {
	b.InitOnce(func() {
		if err := metricsutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		b.ImportBackendConfig = conf
		b.remoteReadTargets = make(map[string]*remoteread.Target)
	})
}

func (b *ImportBackend) AddTarget(ctx context.Context, request *remoteread.TargetAddRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	b.remoteReadTargetMu.Lock()
	defer b.remoteReadTargetMu.Unlock()

	targetId := getIdFromTargetMeta(request.Target.Meta)

	if _, found := b.remoteReadTargets[targetId]; found {
		return nil, targetAlreadyExistsError(targetId)
	}

	if request.Target.Status == nil {
		request.Target.Status = &remoteread.TargetStatus{
			Progress: &remoteread.TargetProgress{},
			Message:  "",
			State:    remoteread.TargetState_Running,
		}
	}

	b.remoteReadTargets[targetId] = request.Target

	b.Logger.With(
		"cluster", request.Target.Meta.ClusterId,
		"target", request.Target.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("added new target")

	return &emptypb.Empty{}, nil
}

func (b *ImportBackend) EditTarget(ctx context.Context, request *remoteread.TargetEditRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	status, err := b.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
		Meta: request.Meta,
	})

	if err != nil {
		return nil, fmt.Errorf("could not check on target status: %w", err)
	}

	if status.State == remoteread.TargetState_Running {
		return nil, fmt.Errorf("can not edit running target")
	}

	b.remoteReadTargetMu.Lock()
	defer b.remoteReadTargetMu.Unlock()

	diff := request.TargetDiff
	targetId := getIdFromTargetMeta(request.Meta)

	target, found := b.remoteReadTargets[targetId]
	if !found {
		return nil, targetDoesNotExistError(targetId)
	}

	if diff.Name != "" {
		target.Meta.Name = diff.Name
		newTargetId := getIdFromTargetMeta(target.Meta)

		if _, found := b.remoteReadTargets[newTargetId]; found {
			return nil, targetAlreadyExistsError(diff.Name)
		}

		delete(b.remoteReadTargets, targetId)
		b.remoteReadTargets[newTargetId] = target
	}

	if diff.Endpoint != "" {
		target.Spec.Endpoint = diff.Endpoint
	}

	b.Logger.With(
		"cluster", request.Meta.ClusterId,
		"target", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("edited target")

	return &emptypb.Empty{}, nil
}

func (b *ImportBackend) RemoveTarget(ctx context.Context, request *remoteread.TargetRemoveRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	status, err := b.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
		Meta: request.Meta,
	})

	if err != nil {
		return nil, fmt.Errorf("could not check on target status: %w", err)
	}

	if status.State == remoteread.TargetState_Running {
		return nil, fmt.Errorf("can not edit running target")
	}

	b.remoteReadTargetMu.Lock()
	defer b.remoteReadTargetMu.Unlock()

	targetId := getIdFromTargetMeta(request.Meta)

	if _, found := b.remoteReadTargets[targetId]; !found {
		return nil, targetDoesNotExistError(request.Meta.Name)
	}

	delete(b.remoteReadTargets, targetId)

	b.Logger.With(
		"cluster", request.Meta.ClusterId,
		"target", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("removed target")

	return &emptypb.Empty{}, nil
}

func (b *ImportBackend) ListTargets(ctx context.Context, request *remoteread.TargetListRequest) (*remoteread.TargetList, error) {
	b.WaitForInit()

	b.remoteReadTargetMu.RLock()
	defer b.remoteReadTargetMu.RUnlock()

	inner := make([]*remoteread.Target, 0, len(b.remoteReadTargets))
	innerMu := sync.RWMutex{}

	eg, ctx := errgroup.WithContext(ctx)
	for _, target := range b.remoteReadTargets {
		if request.ClusterId == "" || request.ClusterId == target.Meta.ClusterId {
			target := target
			eg.Go(func() error {
				newStatus, err := b.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{Meta: target.Meta})
				if err != nil {
					b.Logger.Infof("could not get newStatus for target '%s/%s': %s", target.Meta.ClusterId, target.Meta.Name, err)
					newStatus.State = remoteread.TargetState_Unknown
				}

				target.Status = newStatus

				innerMu.Lock()
				inner = append(inner, target)
				innerMu.Unlock()

				return nil
			})
		}
	}

	if err := eg.Wait(); err != nil {
		b.Logger.Errorf("error waiting for status to update: %s", err)
	}

	list := &remoteread.TargetList{Targets: inner}

	return list, nil
}

func (b *ImportBackend) Start(ctx context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	if b.Delegate == nil {
		return nil, fmt.Errorf("encountered nil delegate")
	}

	b.remoteReadTargetMu.RLock()
	defer b.remoteReadTargetMu.RUnlock()

	targetId := getIdFromTargetMeta(request.Target.Meta)

	// agent needs the full target but cli will ony have access to remoteread.TargetMeta values (clusterId, name, etc)
	// so we need to replace the naive request target
	target, found := b.remoteReadTargets[targetId]
	if !found {
		return nil, targetDoesNotExistError(targetId)
	}

	request.Target = target

	_, err := b.Delegate.WithTarget(&corev1.Reference{Id: request.Target.Meta.ClusterId}).Start(ctx, request)

	if err != nil {
		b.Logger.With(
			"cluster", request.Target.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Target.Meta.Name,
			zap.Error(err),
		).Error("failed to start target")

		return nil, err
	}

	b.Logger.With(
		"cluster", request.Target.Meta.ClusterId,
		"capability", wellknown.CapabilityMetrics,
		"target", request.Target.Meta.Name,
	).Info("target started")

	return &emptypb.Empty{}, nil
}

func (b *ImportBackend) Stop(ctx context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	b.WaitForInit()

	if b.Delegate == nil {
		return nil, fmt.Errorf("encountered nil delegate")
	}

	_, err := b.Delegate.WithTarget(&corev1.Reference{Id: request.Meta.ClusterId}).Stop(ctx, request)

	if err != nil {
		b.Logger.With(
			"cluster", request.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Meta.Name,
			zap.Error(err),
		).Error("failed to stop target")

		return nil, err
	}

	b.Logger.With(
		"cluster", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
		"target", request.Meta.Name,
	).Info("target stopped")

	return &emptypb.Empty{}, nil
}

func (b *ImportBackend) GetTargetStatus(ctx context.Context, request *remoteread.TargetStatusRequest) (*remoteread.TargetStatus, error) {
	b.WaitForInit()

	targetId := getIdFromTargetMeta(request.Meta)

	b.remoteReadTargetMu.RLock()
	defer b.remoteReadTargetMu.RUnlock()
	if _, found := b.remoteReadTargets[targetId]; !found {
		return nil, fmt.Errorf("target '%s/%s' does not exist", request.Meta.ClusterId, request.Meta.Name)
	}

	newStatus, err := b.Delegate.WithTarget(&corev1.Reference{Id: request.Meta.ClusterId}).GetTargetStatus(ctx, request)

	if err != nil {
		if strings.Contains(err.Error(), "target not found") {
			return &remoteread.TargetStatus{
				State: remoteread.TargetState_NotRunning,
			}, nil
		}

		b.Logger.With(
			"cluster", request.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Meta.Name,
			zap.Error(err),
		).Error("failed to get target status")

		return nil, err
	}

	return newStatus, nil
}

func (b *ImportBackend) Discover(ctx context.Context, request *remoteread.DiscoveryRequest) (*remoteread.DiscoveryResponse, error) {
	b.WaitForInit()
	response, err := b.Delegate.WithBroadcastSelector(&corev1.ClusterSelector{
		ClusterIDs: request.ClusterIds,
	}, func(reply interface{}, responses *streamv1.BroadcastReplyList) error {
		discoveryReply := reply.(*remoteread.DiscoveryResponse)
		discoveryReply.Entries = make([]*remoteread.DiscoveryEntry, 0)

		for _, response := range responses.Responses {
			discoverResponse := &remoteread.DiscoveryResponse{}

			if err := proto.Unmarshal(response.Reply.GetResponse().Response, discoverResponse); err != nil {
				b.Logger.Errorf("failed to unmarshal for aggregated DiscoveryResponse: %s", err)
			}

			// inject the cluster id gateway-side
			lo.Map(discoverResponse.Entries, func(entry *remoteread.DiscoveryEntry, _ int) *remoteread.DiscoveryEntry {
				entry.ClusterId = response.Ref.Id
				return entry
			})

			discoveryReply.Entries = append(discoveryReply.Entries, discoverResponse.Entries...)
		}

		return nil
	}).Discover(ctx, request)

	if err != nil {
		b.Logger.With(
			"capability", wellknown.CapabilityMetrics,
			zap.Error(err),
		).Error("failed to run import discovery")

		return nil, err
	}

	return response, nil
}

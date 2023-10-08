package services

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gogo/status"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
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

type RemoteReadServer struct {
	Context types.StreamServiceContext `option:"context"`

	// the stored remoteread.Target should never have their status populated
	remoteReadTargetMu sync.RWMutex
	remoteReadTargets  map[string]*remoteread.Target
}

// Activate implements types.Service
func (m *RemoteReadServer) Activate() error {
	return nil
}

// StreamServices implements types.StreamService
func (s *RemoteReadServer) StreamServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[remoteread.RemoteReadGatewayServer](&remoteread.RemoteReadGateway_ServiceDesc, s),
	}
}

func (m *RemoteReadServer) AddTarget(_ context.Context, request *remoteread.TargetAddRequest) (*emptypb.Empty, error) {
	m.remoteReadTargetMu.Lock()
	defer m.remoteReadTargetMu.Unlock()

	targetId := getIdFromTargetMeta(request.Target.Meta)

	if _, found := m.remoteReadTargets[targetId]; found {
		return nil, targetAlreadyExistsError(targetId)
	}

	if request.Target.Status == nil {
		request.Target.Status = &remoteread.TargetStatus{
			Progress: &remoteread.TargetProgress{},
			Message:  "",
			State:    remoteread.TargetState_Running,
		}
	}

	m.remoteReadTargets[targetId] = request.Target

	m.Context.Logger().With(
		"cluster", request.Target.Meta.ClusterId,
		"target", request.Target.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("added new target")

	return &emptypb.Empty{}, nil
}

func (m *RemoteReadServer) EditTarget(ctx context.Context, request *remoteread.TargetEditRequest) (*emptypb.Empty, error) {
	status, err := m.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
		Meta: request.Meta,
	})

	if err != nil {
		return nil, fmt.Errorf("could not check on target status: %w", err)
	}

	if status.State == remoteread.TargetState_Running {
		return nil, fmt.Errorf("can not edit running target")
	}

	m.remoteReadTargetMu.Lock()
	defer m.remoteReadTargetMu.Unlock()

	diff := request.TargetDiff
	targetId := getIdFromTargetMeta(request.Meta)

	target, found := m.remoteReadTargets[targetId]
	if !found {
		return nil, targetDoesNotExistError(targetId)
	}

	if diff.Name != "" {
		target.Meta.Name = diff.Name
		newTargetId := getIdFromTargetMeta(target.Meta)

		if _, found := m.remoteReadTargets[newTargetId]; found {
			return nil, targetAlreadyExistsError(diff.Name)
		}

		delete(m.remoteReadTargets, targetId)
		m.remoteReadTargets[newTargetId] = target
	}

	if diff.Endpoint != "" {
		target.Spec.Endpoint = diff.Endpoint
	}

	m.Context.Logger().With(
		"cluster", request.Meta.ClusterId,
		"target", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("edited target")

	return &emptypb.Empty{}, nil
}

func (m *RemoteReadServer) RemoveTarget(ctx context.Context, request *remoteread.TargetRemoveRequest) (*emptypb.Empty, error) {
	status, err := m.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{
		Meta: request.Meta,
	})

	if err != nil {
		return nil, fmt.Errorf("could not check on target status: %w", err)
	}

	if status.State == remoteread.TargetState_Running {
		return nil, fmt.Errorf("can not edit running target")
	}

	m.remoteReadTargetMu.Lock()
	defer m.remoteReadTargetMu.Unlock()

	targetId := getIdFromTargetMeta(request.Meta)

	if _, found := m.remoteReadTargets[targetId]; !found {
		return nil, targetDoesNotExistError(request.Meta.Name)
	}

	delete(m.remoteReadTargets, targetId)

	m.Context.Logger().With(
		"cluster", request.Meta.ClusterId,
		"target", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
	).Infof("removed target")

	return &emptypb.Empty{}, nil
}

func (m *RemoteReadServer) ListTargets(ctx context.Context, request *remoteread.TargetListRequest) (*remoteread.TargetList, error) {
	m.remoteReadTargetMu.RLock()
	defer m.remoteReadTargetMu.RUnlock()

	inner := make([]*remoteread.Target, 0, len(m.remoteReadTargets))
	innerMu := sync.RWMutex{}

	eg, ctx := errgroup.WithContext(ctx)
	for _, target := range m.remoteReadTargets {
		if request.ClusterId == "" || request.ClusterId == target.Meta.ClusterId {
			target := target
			eg.Go(func() error {
				newStatus, err := m.GetTargetStatus(ctx, &remoteread.TargetStatusRequest{Meta: target.Meta})
				if err != nil {
					m.Context.Logger().Infof("could not get newStatus for target '%s/%s': %s", target.Meta.ClusterId, target.Meta.Name, err)
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
		m.Context.Logger().Errorf("error waiting for status to update: %s", err)
	}

	list := &remoteread.TargetList{Targets: inner}

	return list, nil
}

func (m *RemoteReadServer) GetTargetStatus(ctx context.Context, request *remoteread.TargetStatusRequest) (*remoteread.TargetStatus, error) {
	targetId := getIdFromTargetMeta(request.Meta)

	m.remoteReadTargetMu.RLock()
	defer m.remoteReadTargetMu.RUnlock()
	if _, found := m.remoteReadTargets[targetId]; !found {
		return nil, fmt.Errorf("target '%s/%s' does not exist", request.Meta.ClusterId, request.Meta.Name)
	}

	newStatus, err := m.Context.Delegate().WithTarget(&corev1.Reference{Id: request.Meta.ClusterId}).GetTargetStatus(ctx, request)

	if err != nil {
		if strings.Contains(err.Error(), "target not found") {
			return &remoteread.TargetStatus{
				State: remoteread.TargetState_NotRunning,
			}, nil
		}

		m.Context.Logger().With(
			"cluster", request.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Meta.Name,
			zap.Error(err),
		).Error("failed to get target status")

		return nil, err
	}

	return newStatus, nil
}

func (m *RemoteReadServer) Start(ctx context.Context, request *remoteread.StartReadRequest) (*emptypb.Empty, error) {
	m.remoteReadTargetMu.RLock()
	defer m.remoteReadTargetMu.RUnlock()

	targetId := getIdFromTargetMeta(request.Target.Meta)

	// agent needs the full target but cli will ony have access to remoteread.TargetMeta values (clusterId, name, etc)
	// so we need to replace the naive request target
	target, found := m.remoteReadTargets[targetId]
	if !found {
		return nil, targetDoesNotExistError(targetId)
	}

	request.Target = target

	_, err := m.Context.Delegate().WithTarget(&corev1.Reference{Id: request.Target.Meta.ClusterId}).Start(ctx, request)

	if err != nil {
		m.Context.Logger().With(
			"cluster", request.Target.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Target.Meta.Name,
			zap.Error(err),
		).Error("failed to start target")

		return nil, err
	}

	m.Context.Logger().With(
		"cluster", request.Target.Meta.ClusterId,
		"capability", wellknown.CapabilityMetrics,
		"target", request.Target.Meta.Name,
	).Info("target started")

	return &emptypb.Empty{}, nil
}

func (m *RemoteReadServer) Stop(ctx context.Context, request *remoteread.StopReadRequest) (*emptypb.Empty, error) {
	_, err := m.Context.Delegate().WithTarget(&corev1.Reference{Id: request.Meta.ClusterId}).Stop(ctx, request)

	if err != nil {
		m.Context.Logger().With(
			"cluster", request.Meta.ClusterId,
			"capability", wellknown.CapabilityMetrics,
			"target", request.Meta.Name,
			zap.Error(err),
		).Error("failed to stop target")

		return nil, err
	}

	m.Context.Logger().With(
		"cluster", request.Meta.Name,
		"capability", wellknown.CapabilityMetrics,
		"target", request.Meta.Name,
	).Info("target stopped")

	return &emptypb.Empty{}, nil
}

func (m *RemoteReadServer) Discover(ctx context.Context, request *remoteread.DiscoveryRequest) (*remoteread.DiscoveryResponse, error) {
	response, err := m.Context.Delegate().WithBroadcastSelector(&corev1.ClusterSelector{
		ClusterIDs: request.ClusterIds,
	}, func(reply interface{}, responses *streamv1.BroadcastReplyList) error {
		discoveryReply := reply.(*remoteread.DiscoveryResponse)
		discoveryReply.Entries = make([]*remoteread.DiscoveryEntry, 0)

		for _, response := range responses.Responses {
			discoverResponse := &remoteread.DiscoveryResponse{}

			if err := proto.Unmarshal(response.Reply.GetResponse().Response, discoverResponse); err != nil {
				m.Context.Logger().Errorf("failed to unmarshal for aggregated DiscoveryResponse: %s", err)
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
		m.Context.Logger().With(
			"capability", wellknown.CapabilityMetrics,
			zap.Error(err),
		).Error("failed to run import discovery")

		return nil, err
	}

	return response, nil
}

func init() {
	types.Services.Register("Remote Read Stream Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &RemoteReadServer{
			remoteReadTargets: make(map[string]*remoteread.Target),
		}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}

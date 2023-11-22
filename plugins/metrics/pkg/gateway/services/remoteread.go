package services

import (
	"context"
	"fmt"
	"strings"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	Context types.ManagementServiceContext `option:"context"`

	// the stored remoteread.Target should never have their status populated
	remoteReadTargetMu sync.RWMutex
	remoteReadTargets  map[string]*remoteread.Target
}

// Activate implements types.Service
func (m *RemoteReadServer) Activate() error {
	defer m.Context.SetServingStatus(remoteread.RemoteReadGateway_ServiceDesc.ServiceName, managementext.Serving)
	return nil
}

// ManagementServices implements types.ManagementService
func (m *RemoteReadServer) ManagementServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[remoteread.RemoteReadGatewayServer](&remoteread.RemoteReadGateway_ServiceDesc, m),
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
	).Info("added new target")

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
	).Info("edited target")

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
	).Info("removed target")

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
					m.Context.Logger().Info(fmt.Sprintf("could not get newStatus for target '%s/%s': %s", target.Meta.ClusterId, target.Meta.Name, err))
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
		m.Context.Logger().With(logger.Err(err)).Error("error waiting for status to update")
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
			logger.Err(err),
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
			logger.Err(err),
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
			logger.Err(err),
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
	targets := make([]*corev1.Reference, 0, len(request.ClusterIds))
	for _, clusterId := range request.ClusterIds {
		targets = append(targets, &corev1.Reference{Id: clusterId})
	}

	aggregatedResponses := &remoteread.DiscoveryResponse{}
	_, err := m.Context.Delegate().WithBroadcastSelector(&streamv1.TargetSelector{
		Targets: targets,
	}, func(target *corev1.Reference, resp *remoteread.DiscoveryResponse, err error) {
		for _, entry := range resp.GetEntries() {
			// inject the cluster id gateway-side
			entry.ClusterId = target.GetId()
		}
		aggregatedResponses.Entries = append(aggregatedResponses.Entries, resp.GetEntries()...)
	}).Discover(ctx, request)

	if err != nil {
		m.Context.Logger().With(
			"capability", wellknown.CapabilityMetrics,
			logger.Err(err),
		).Error("failed to run import discovery")

		return nil, err
	}

	return aggregatedResponses, nil
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

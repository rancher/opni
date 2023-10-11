package services

import (
	"context"

	amcfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/configutil"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc"
)

type RoutingStorageService interface {
	alertingv2.RoutingServiceServer
	storage.KeyValueStoreT[*amcfg.Config]
	Impl() grpc.ServiceDesc
}

type routingService struct {
	alertingv2.UnsafeRoutingServiceServer
	storage.KeyValueStoreT[*amcfg.Config]
}

func toLabelSet(labels map[string]string) model.LabelSet {
	res := model.LabelSet{}
	for k, v := range labels {
		res[model.LabelName(k)] = model.LabelValue(v)
	}
	return res
}

func (r *routingService) Matches(ctx context.Context, req *alertingv2.AlertPayload) (*alertingv2.RouteResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	res := &alertingv2.RouteResponse{
		Receivers: map[string]*alertingv2.Matched{},
	}
	runMatchersOn := map[string]*amcfg.Config{}
	for _, cluster := range req.Clusters {
		config, err := r.KeyValueStoreT.Get(ctx, cluster.GetId())
		if err != nil {
			res.Receivers[cluster.GetId()] = &alertingv2.Matched{}
		} else {
			runMatchersOn[cluster.GetId()] = config
		}
	}
	for clusterId, config := range runMatchersOn {
		routes := dispatch.NewRoute(config.Route, nil)
		matchedRoutes := routes.Match(configutil.ToLabelSet(req.Labels))
		matched := &alertingv2.Matched{
			Names: []string{},
		}
		for _, route := range matchedRoutes {
			receiver := route.RouteOpts.Receiver
			matched.Names = append(matched.Names, receiver)
		}
		res.Receivers[clusterId] = matched
	}
	return res, nil
}

func (r *routingService) Impl() grpc.ServiceDesc {
	return alertingv2.RoutingService_ServiceDesc
}

var _ RoutingStorageService = &routingService{}

func NewRoutingService(
	store storage.KeyValueStoreT[*amcfg.Config],
) RoutingStorageService {
	return &routingService{
		KeyValueStoreT: store,
	}
}

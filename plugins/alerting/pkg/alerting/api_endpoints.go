package alerting

import (
	"context"
	"path"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/config"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

const endpointPrefix = "/alerting/endpoints"

var locker = sync.Mutex{}

func configFromBackend(backend backend.RuntimeEndpointBackend, ctx context.Context, p *Plugin) (*config.ConfigMapData, error) {
	locker.Lock()
	defer locker.Unlock()
	options, err := p.AlertingOptions.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	rawConfig, err := backend.Fetch(ctx, p.Logger, options, "alertmanager.yaml")
	if err != nil {
		return nil, err
	}
	config, err := config.NewConfigMapDataFrom(rawConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func applyConfigToBackend(
	backend backend.RuntimeEndpointBackend,
	ctx context.Context,
	p *Plugin,
	config *config.ConfigMapData,
	updatedConditionId string,
) error {
	options, err := p.AlertingOptions.GetContext(ctx)
	if err != nil {
		return err
	}

	err = backend.Put(ctx, p.Logger, options, "alertmanager.yaml", config)
	if err != nil {
		return err
	}
	//p.Logger.Debug("Sleeping")
	//time.Sleep(time.Second * 10) // FIXME: configmap needs to reload on disk before we reload
	//p.Logger.Debug("Done sleeping")
	err = backend.Reload(ctx, p.Logger, options, updatedConditionId)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := uuid.New().String()
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, newId), req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	return storage.AlertEndpoint.Get(ctx, path.Join(endpointPrefix, ref.Id))
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	_, err = storage.AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	if err := storage.AlertEndpoint.Put(ctx, path.Join(endpointPrefix, req.Id.Id), req.UpdateAlert); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	ids, endpoints, err := listWithKeys(ctx, storage.AlertEndpoint, endpointPrefix)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(endpoints) {
		return nil, err
	}
	items := []*alertingv1alpha.AlertEndpointWithId{}
	for idx := range ids {
		items = append(items, &alertingv1alpha.AlertEndpointWithId{
			Id:       &corev1.Reference{Id: ids[idx]},
			Endpoint: endpoints[idx],
		})
	}
	return &alertingv1alpha.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := storage.AlertEndpoint.Delete(ctx, path.Join(endpointPrefix, ref.Id)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	lg := p.Logger.With("Handler", "TestAlertEndpoint")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))

	// - Create dummy Endpoint id
	dummyConditionId := "test-" + uuid.New().String()
	lg.Debugf("dummy id : %s", dummyConditionId)
	impl := req.GetImpl()
	impl.InitialDelay = durationpb.New(time.Duration(time.Second * 0))
	createImpl := &alertingv1alpha.CreateImplementation{
		ConditionId:    &corev1.Reference{Id: dummyConditionId}, // is used as a unique identifier
		EndpointId:     &corev1.Reference{Id: ""},               // should never be used
		Implementation: req.GetImpl(),
	}
	recv, err := processEndpointDetails(dummyConditionId, createImpl, req.GetEndpointInfo())
	if err != nil {
		return nil, err
	}
	pluginBackend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch backend within timeout : %s", err)
		return nil, err
	}
	options, err := p.AlertingOptions.GetContext(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return nil, err
	}
	// interact with pluginBackend
	config, err := configFromBackend(pluginBackend, ctx, p)
	if err != nil {
		return nil, err
	}
	lg.Debugf("received config from pluginBackend %v", config)
	config.AppendReceiver(recv)
	config.AppendRoute(recv, createImpl)
	err = applyConfigToBackend(pluginBackend, ctx, p, config, dummyConditionId)
	if err != nil {
		return nil, err
	}
	lg.Debug("done reloading")
	// - Trigger it using httpv2 api
	e := options.GetWorkerEndpoint()
	alert := &backend.PostableAlert{}
	alert.WithCondition(dummyConditionId)
	var alerts []*backend.PostableAlert
	alerts = append(alerts, alert)
	lg.Debugf("sending alert to alertmanager : %v, %v", alert.Annotations, alert.Labels)
	_, resp, err := backend.PostAlert(context.Background(), e, alerts)
	if err != nil {
		lg.Errorf("Error while posting alert : %s", err)
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, shared.WithInternalServerErrorf("failed to send alert to alertmanager")
	}
	lg.Debugf("Got response %s from alertmanager", resp.Status)
	// We need to check that endpoint is deleted after a certain amount of time
	// - Delete dummy Endpoint
	go func() {
		time.Sleep(time.Second)
		// this cannot be done in the same context as the above call, otherwise the deadline will be exceeded
		if _, err := p.DeleteEndpointImplementation(context.TODO(), &corev1.Reference{Id: dummyConditionId}); err != nil {
			lg.Errorf("delete test implementation failed with %s", err.Error())
		}
	}()
	return &alertingv1alpha.TestAlertEndpointResponse{}, nil
}

func processEndpointDetails(
	conditionId string,
	req *alertingv1alpha.CreateImplementation,
	endpointDetails *alertingv1alpha.AlertEndpoint,
) (*config.Receiver, error) {
	if s := endpointDetails.GetSlack(); s != nil {
		recv, err := config.NewSlackReceiver(conditionId, s)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = config.WithSlackImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	if e := endpointDetails.GetEmail(); e != nil {
		recv, err := config.NewEmailReceiver(conditionId, e)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = config.WithEmailImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	return nil, validation.Error("Unhandled endpoint/implementation details")
}

// Called from CreateAlertCondition
func (p *Plugin) CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))
	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.EndpointId.Id))
	if err != nil {
		return nil, err
	}
	conditionId := req.ConditionId.Id
	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}
	config.AppendReceiver(recv)
	config.AppendRoute(recv, req)
	err = applyConfigToBackend(backend, ctx, p, config, conditionId)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from UpdateAlertCondition
func (p *Plugin) UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))

	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.EndpointId.Id))
	if err != nil {
		return nil, err
	}
	conditionId := req.ConditionId.Id
	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}
	//note: don't need to update routes after this since they hold a ref to the original condition id
	err = config.UpdateReceiver(conditionId, recv)
	if err != nil {
		return nil, err
	}
	err = config.UpdateRoute(conditionId, recv, req)
	if err != nil {
		return nil, err
	}
	err = applyConfigToBackend(backend, ctx, p, config, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from DeleteAlertCondition
// Id must be a conditionId
func (p *Plugin) DeleteEndpointImplementation(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	ctx, _ = setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))
	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	if err := config.DeleteReceiver(req.Id); err != nil {
		return nil, err
	}
	if err := config.DeleteRoute(req.Id); err != nil {
		return nil, err
	}
	err = applyConfigToBackend(backend, ctx, p, config, req.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

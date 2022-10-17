package drivers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/gogo/status"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Implementation of the AlertingDynamic Server for
// AlertingManager

var mu = sync.Mutex{}

func (a *AlertingManager) ConfigFromBackend(ctx context.Context) (*routing.RoutingTree, *routing.OpniInternalRouting, error) {
	mu.Lock()
	defer mu.Unlock()
	rawConfig, err := a.Fetch(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, nil, err
	}
	config, err := routing.NewRoutingTreeFrom(rawConfig.RawAlertManagerConfig)
	if err != nil {
		return nil, nil, err
	}
	internal, err := routing.NewOpniInternalRoutingFrom(rawConfig.RawInternalRouting)
	if err != nil {
		return nil, nil, err
	}
	return config, internal, nil
}

func (a *AlertingManager) ApplyConfigToBackend(
	ctx context.Context,
	config *routing.RoutingTree,
	internal *routing.OpniInternalRouting,
	updatedConditionId string,
) error {
	mu.Lock()
	defer mu.Unlock()
	rawAlertManagerData, err := config.Marshal()
	if err != nil {
		return err
	}
	rawInternalRoutingData, err := internal.Marshal()
	if err != nil {
		return err
	}
	_, err = a.Update(ctx, &alertops.AlertingConfig{
		RawAlertManagerConfig: string(rawAlertManagerData),
		RawInternalRouting:    string(rawInternalRoutingData),
	})
	if err != nil {
		return err
	}
	_, err = a.Reload(ctx, &alertops.ReloadInfo{
		UpdatedKey: updatedConditionId,
	})
	return nil
}

func (a *AlertingManager) Fetch(ctx context.Context, _ *emptypb.Empty) (*alertops.AlertingConfig, error) {
	lg := a.Logger.With("action", "Fetch")
	name := a.AlertingOptions.ConfigMap
	namespace := a.gatewayRef.Namespace
	cfgMap := &k8scorev1.ConfigMap{}
	err := a.k8sClient.Get(
		ctx,
		client.ObjectKey{
			Name: name, Namespace: namespace},
		cfgMap)

	if err != nil || cfgMap == nil {
		msg := fmt.Sprintf("K8s runtime error, config map: %s/%s not found: %s",
			namespace,
			name,
			err)
		lg.Error(msg)
		returnErr := shared.WithInternalServerError(
			msg,
		)
		return nil, returnErr
	}
	if _, ok := cfgMap.Data[a.configKey]; !ok {
		msg := fmt.Sprintf("K8s runtime error, config map : %s key : %s not found",
			name,
			a.configKey)
		lg.Error(msg)
		return nil, shared.WithInternalServerError(
			msg,
		)
	}
	if _, ok := cfgMap.Data[a.internalRoutingKey]; !ok {
		msg := fmt.Sprintf("K8s runtime error, config map : %s key : %s not found",
			name,
			a.internalRoutingKey)
		lg.Error(msg)
		return nil, shared.WithInternalServerError(
			msg,
		)
	}
	return &alertops.AlertingConfig{
		RawAlertManagerConfig: cfgMap.Data[a.configKey],
		RawInternalRouting:    cfgMap.Data[a.internalRoutingKey],
	}, nil
}

func (a *AlertingManager) Update(ctx context.Context, conf *alertops.AlertingConfig) (*emptypb.Empty, error) {
	lg := a.Logger.With("action", "Update")
	a.configPersistMu.Lock()
	defer a.configPersistMu.Unlock()
	cfgStruct := &routing.RoutingTree{}
	err := cfgStruct.Parse(conf.RawAlertManagerConfig)
	if err != nil {
		return nil, err
	}
	loopError := backend.ReconcileInvalidStateLoop(
		time.Duration(time.Second*10),
		cfgStruct,
		lg)
	if loopError != nil {
		return nil, shared.WithInternalServerError(fmt.Sprintf("failed to reconcile config : %s", loopError))
	}

	mutator := func(object client.Object) error {
		switch gateway := object.(type) {
		case *corev1beta1.Gateway:
			gateway.Spec.Alerting.RawAlertManagerConfig = conf.RawAlertManagerConfig
			gateway.Spec.Alerting.RawInternalRouting = conf.RawInternalRouting
			return nil
		case *v1beta2.Gateway:
			gateway.Spec.Alerting.RawAlertManagerConfig = conf.RawAlertManagerConfig
			gateway.Spec.Alerting.RawInternalRouting = conf.RawInternalRouting
			return nil
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
	}

	err = retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		existing, err := a.newOpniGateway()
		if err != nil {
			return err
		}
		err = a.k8sClient.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		var clone client.Object
		switch gateway := existing.(type) {
		case *corev1beta1.Gateway:
			clone = gateway.DeepCopyObject().(client.Object)
		case *v1beta2.Gateway:
			clone = gateway.DeepCopyObject().(client.Object)
		default:
			return fmt.Errorf("unkown gateway type %T", gateway)
		}
		if err := mutator(clone); err != nil {
			return err
		}
		cmp, err := patch.DefaultPatchMaker.Calculate(existing, clone,
			patch.IgnoreStatusFields(),
			patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus(),
			patch.IgnorePDBSelector(),
		)
		if err == nil {
			if cmp.IsEmpty() {
				return status.Error(codes.FailedPrecondition, "no changes to apply")
			}
		}
		return a.k8sClient.Update(ctx, clone)
	})
	if err != nil {
		return nil, err
	}

	// TODO : check config map is updated before returning

	return &emptypb.Empty{}, nil
}

func (a *AlertingManager) GetStatus(ctx context.Context, _ *emptypb.Empty) (*alertops.DynamicStatus, error) {
	// check it has been reloaded succesfully and is running
	return nil, nil
}

func (a *AlertingManager) Reload(ctx context.Context, reloadInfo *alertops.ReloadInfo) (*emptypb.Empty, error) {
	lg := a.Logger.With("alerting-backend", "k8s", "action", "reload")

	// TODO: FIXME: MEGA FIXME: this reload only implemnts logic for TestEndpoint API

	reloadEndpoints := []string{}
	// RELOAD the controller!!!
	controllerSvcEndpoint := a.AlertingOptions.GetControllerEndpoint()
	reloadEndpoints = append(reloadEndpoints, controllerSvcEndpoint)
	// RELOAD the workers
	name := a.AlertingOptions.WorkerNodesService
	namespace := a.AlertingOptions.Namespace
	workersEndpoints := k8scorev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := a.k8sClient.Get(ctx, client.ObjectKeyFromObject(&workersEndpoints), &workersEndpoints)
	if err != nil {
		return nil, err
	}
	if len(workersEndpoints.Subsets) > 0 {
		addresses := workersEndpoints.Subsets[0].Addresses
		for _, address := range addresses {
			reloadEndpoints = append(reloadEndpoints, fmt.Sprintf("%s:%d", address.IP, a.AlertingOptions.WorkerNodePort))
		}
	}
	wg := sync.WaitGroup{}
	errors := &sharedErrors{}

	for _, endpoint := range reloadEndpoints {
		wg.Add(1)
		endpoint := endpoint

		go func() {
			defer wg.Done()
			reloadClient := &backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/reload",
				Verb:     backend.POST,
			}
			webClient := &backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/ready",
				Verb:     backend.GET,
			}
			receiverClient := (&backend.AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/receivers",
				Verb:     backend.GET,
			}).WithHttpV2()

			// reload logic
			numReloads := 0
			numNotFound := 0

			retrier := backoffv2.Exponential(
				backoffv2.WithMaxRetries(10),
				backoffv2.WithMinInterval(2*time.Second),
				backoffv2.WithMaxInterval(5*time.Second),
				backoffv2.WithMultiplier(1.2),
			)
			b := retrier.Start(ctx)
			for backoffv2.Continue(b) { //FIXME: this logic is janky
				numReloads += 1
				resp, err := http.Post(reloadClient.ConstructHTTP(), "application/json", nil)
				if err != nil {
					lg.Errorf("failed to reload alertmanager %s : %s", reloadClient.Endpoint, err)
					appendError(errors, err)
					return
				}
				if resp.StatusCode != 200 {
					msg := fmt.Sprintf("failed to reload alertmanager %s successfully : %s", reloadClient.Endpoint, resp.Status)
					lg.Errorf(msg)
					appendError(errors, fmt.Errorf(msg))
					return
				}
				receiverResponse, err := http.Get(receiverClient.ConstructHTTP())
				if err != nil {
					msg := fmt.Sprintf("failed to fetch alertmanager receivers manually for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					appendError(errors, err)
				}
				if receiverResponse.StatusCode != 200 {
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					appendError(errors, err)
				}

				body, err := io.ReadAll(receiverResponse.Body)
				if err != nil {
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					appendError(errors, err)
				}
				result := gjson.Get(string(body), "#.name")
				found := false
				for _, receiver := range result.Array() {
					if receiver.String() == reloadInfo.UpdatedKey {
						found = true
						break
					}
				}
				if !found {
					numNotFound += 1
				} else {
					break
				}
			}

			if numNotFound > 0 {
				lg.Warnf("Reloaded %s %d times, but receiver not found %d times", reloadClient.Endpoint, numReloads, numNotFound)
				if numNotFound == 100 {
					lg.Warnf("Reload likely failed for %s", reloadClient.Endpoint)
				}
			}
			//FIXME: retrier backoff
			for i := 0; i < 10; i++ {
				lg.Debugf("Checking alertmanager %s is ready ...", webClient.Endpoint)
				resp, err := http.Get(webClient.ConstructHTTP())
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						lg.Debugf("alertmanager %s is ready ...", webClient.Endpoint)
						return
					} else {
						lg.Warnf("Alert manager %s not ready after reload, retrying...", webClient.Endpoint)
					}
				}
				time.Sleep(time.Second)
			}
		}()
	}
	wg.Wait()
	if len(errors.errors) > 0 {
		return nil, shared.WithInternalServerErrorf("alert backend reload failed %s", strings.Join(func() []string {
			res := []string{}
			for _, e := range errors.errors {
				res = append(res, e.Error())
			}
			return res
		}(), ","))
	}

	return &emptypb.Empty{}, nil
}

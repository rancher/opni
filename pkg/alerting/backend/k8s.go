package backend

import (
	"context"
	"fmt"
	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
	"time"
)

var configPersistMu = &sync.Mutex{}

var _ RuntimeEndpointBackend = &K8sEndpointBackend{}

// K8sEndpointBackend implements alerting.RuntimeEndpointBackend
type K8sEndpointBackend struct {
	Client client.Client
}

func (b *K8sEndpointBackend) Fetch(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) (string, error) {
	lg = lg.With("alerting-backend", "k8s", "action", "get")
	name := options.ConfigMap
	namespace := options.Namespace
	lg.With("Alerting backend", "k8s").Debugf("updating")
	cfgMap := &corev1.ConfigMap{}
	err := b.Client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, cfgMap)

	if err != nil || cfgMap == nil {
		msg := fmt.Sprintf("K8s runtime error, config map: %s/%s not found: %s",
			namespace,
			name,
			err)
		lg.Error(msg)
		returnErr := shared.WithInternalServerError(
			msg,
		)
		return "", returnErr
	}

	if _, ok := cfgMap.Data[key]; !ok {
		msg := fmt.Sprintf("K8s runtime error, config map : %s key : %s not found",
			name,
			key)
		lg.Error(msg)
		return "", shared.WithInternalServerError(
			msg,
		)
	}
	return cfgMap.Data[key], nil
}

func (b *K8sEndpointBackend) Put(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string, newData *routing.RoutingTree) error {
	lg = lg.With("alerting-backend", "k8s", "action", "put")

	configPersistMu.Lock()
	defer configPersistMu.Unlock()
	loopError := ReconcileInvalidStateLoop(time.Duration(time.Second*10), newData, lg)
	if loopError != nil {
		return shared.WithInternalServerError(fmt.Sprintf("failed to reconcile config : %s", loopError))
	}
	applyData, err := newData.Marshal()
	if err != nil {
		lg.Errorf("failed to marshal config data : %s", err)
		return err
	}
	gw := &v1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			//FIXME: need to add gw id to system.go
			Name:      "opni-gateway",
			Namespace: "opni",
		},
	}
	mutator := func(gw *v1beta1.Gateway) {
		gw.Spec.Alerting.RawConfigMap = string(applyData)
	}
	objectKey := client.ObjectKeyFromObject(gw)
	err = retry.OnError(retry.DefaultBackoff, k8serrors.IsConflict, func() error {
		existing := &v1beta1.Gateway{}
		err := b.Client.Get(ctx, objectKey, existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone)
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
		return b.Client.Update(ctx, clone)
	})
	if err != nil {
		msg := fmt.Sprintf("failed to update gateway : %s", err)
		lg.Error(msg)
		return shared.WithInternalServerError(msg)
	}
	for i := 0; i < 10; i++ {
		cfgMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      options.ConfigMap,
				Namespace: options.Namespace,
			},
		}
		err := b.Client.Get(ctx, client.ObjectKeyFromObject(cfgMap), cfgMap)
		if err != nil {
			lg.Warnf("config map with name %s, namespace : %s not found")
		}
		if cfgMap.Data[shared.AlertManagerConfigKey] != string(applyData) {
			lg.Warn("New config map data does not yet match the expected data, waiting...")
		} else {
			lg.Debug("Config map data matches expected data, continuing to reload...")
			return nil
		}
		time.Sleep(time.Second)
	}
	lg.Debug("config map may be weird")
	return nil
}

func (b *K8sEndpointBackend) Reload(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) error {
	lg = lg.With("alerting-backend", "k8s", "action", "reload")
	reloadEndpoints := []string{}
	// RELOAD the controller!!!
	controllerSvcEndpoint := options.GetControllerEndpoint()
	reloadEndpoints = append(reloadEndpoints, controllerSvcEndpoint)
	wg := sync.WaitGroup{}
	errMtx := sync.Mutex{}
	var errors []error

	// RELOAD the workers

	name := options.WorkerNodesService
	namespace := options.Namespace
	workersEndpoints := corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := b.Client.Get(ctx, client.ObjectKeyFromObject(&workersEndpoints), &workersEndpoints)
	if err != nil {
		return err
	}
	addresses := workersEndpoints.Subsets[0].Addresses
	for _, address := range addresses {
		reloadEndpoints = append(reloadEndpoints, fmt.Sprintf("%s:%d", address.IP, options.WorkerNodePort))
	}

	for _, endpoint := range reloadEndpoints {
		wg.Add(1)
		//addr := addr
		go func() {
			defer wg.Done()
			reloadClient := &AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/reload",
				Verb:     POST,
			}
			webClient := &AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/-/ready",
				Verb:     GET,
			}
			receiverClient := (&AlertManagerAPI{
				Endpoint: endpoint,
				Route:    "/receivers",
				Verb:     GET,
			}).WithHttpV2()

			// reload logic
			numReloads := 0
			numNotFound := 0
			retries := 100
			for i := 0; i < retries; i++ {
				numReloads += 1
				resp, err := http.Post(reloadClient.ConstructHTTP(), "application/json", nil)
				if err != nil {
					errMtx.Lock()
					lg.Errorf("failed to reload alertmanager %s : %s", reloadClient.Endpoint, err)
					errors = append(errors, err)
					errMtx.Unlock()
					return
				}
				if resp.StatusCode != 200 {
					errMtx.Lock()
					msg := fmt.Sprintf("failed to reload alertmanager %s successfully : %s", reloadClient.Endpoint, resp.Status)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
					return
				}
				receiverResponse, err := http.Get(receiverClient.ConstructHTTP())
				if err != nil {
					errMtx.Lock()
					msg := fmt.Sprintf("failed to fetch alertmanager receivers manually for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}
				if receiverResponse.StatusCode != 200 {
					errMtx.Lock()
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}

				body, err := io.ReadAll(receiverResponse.Body)
				if err != nil {
					errMtx.Lock()
					msg := fmt.Sprintf("got unexpected receiver for %s", receiverClient.Endpoint)
					lg.Errorf(msg)
					errors = append(errors, fmt.Errorf(msg))
					errMtx.Unlock()
				}
				result := gjson.Get(string(body), "#.name")
				found := false
				for _, receiver := range result.Array() {
					if receiver.String() == key {
						found = true
						break
					}
				}
				if !found {
					numNotFound += 1
				} else {
					break
				}
				time.Sleep(time.Second * 1)
			}

			if numNotFound > 0 {
				lg.Warnf("Reloaded %s %d times, but receiver not found %d times", reloadClient.Endpoint, numReloads, numNotFound)
				if numNotFound == 100 {
					lg.Warnf("Reload likely failed for %s", reloadClient.Endpoint)
				}
			}

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
	if len(errors) > 0 {
		return shared.WithInternalServerErrorf("alert backend reload failed %s", strings.Join(func() []string {
			res := []string{}
			for _, e := range errors {
				res = append(res, e.Error())
			}
			return res
		}(), ","))
	}
	return nil
}

func (b *K8sEndpointBackend) Port() int {
	return -1
}

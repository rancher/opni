//go:build !minimal

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	"github.com/go-logr/logr"
	"github.com/huandu/xstrings"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources/monitoring"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/samber/lo"
	"github.com/ttacon/chalk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.opni.io,resources=monitoringclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

type CoreMonitoringReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	logger   logr.Logger
	mdClient metadata.Interface
}

func (r *CoreMonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := r.logger

	umc := unstructured.Unstructured{}
	umc.SetGroupVersionKind(corev1beta1.GroupVersion.WithKind("MonitoringCluster"))
	err := r.Get(ctx, req.NamespacedName, &umc)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	op, err := r.Upgrade(ctx, &umc)
	if err != nil {
		lg.Error(err, "failed to upgrade monitoring cluster")
		return ctrl.Result{}, err
	} else if op.ShouldRequeue() {
		if err := r.Update(ctx, &umc); err != nil {
			return k8sutil.RequeueErr(err).Result()
		}
		return op.Result()
	}
	var mc corev1beta1.MonitoringCluster
	jsonData, err := umc.MarshalJSON()
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := json.Unmarshal(jsonData, &mc); err != nil {
		return ctrl.Result{}, err
	}

	rec := monitoring.NewReconciler(ctx, r.Client, &mc)
	result, err := rec.Reconcile()
	if err != nil {
		lg.WithValues(
			"gateway", mc.Name,
			"namespace", mc.Namespace,
		).Error(err, "failed to reconcile monitoring cluster")
		return ctrl.Result{}, err
	}
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CoreMonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = mgr.GetLogger().WithName("monitoring")
	r.Client = mgr.GetClient()
	r.scheme = mgr.GetScheme()
	var err error
	r.mdClient, err = metadata.NewForConfigAndClient(mgr.GetConfig(), mgr.GetHTTPClient())
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1beta1.MonitoringCluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Watches(
			&corev1beta1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(r.findMonitoringClusters),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CoreMonitoringReconciler) findMonitoringClusters(ctx context.Context, gw client.Object) []ctrl.Request {
	// Look up all MonitoringClusters that reference this gateway
	monitoringClusters := &corev1beta1.MonitoringClusterList{}
	err := r.List(ctx, monitoringClusters, client.InNamespace(gw.GetNamespace()))
	if err != nil {
		return []ctrl.Request{}
	}
	var requests []ctrl.Request
	for _, mc := range monitoringClusters.Items {
		if mc.Spec.Gateway.Name == gw.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      mc.Name,
					Namespace: mc.Namespace,
				},
			})
		}
	}
	return requests
}

func (r *CoreMonitoringReconciler) Upgrade(ctx context.Context, umc *unstructured.Unstructured) (k8sutil.RequeueOp, error) {
	currentRevision := corev1beta1.GetMonitoringClusterRevision(umc)
	if currentRevision < 0 {
		corev1beta1.SetMonitoringClusterRevision(umc, 0)
	} else if currentRevision == 0 {
		// we might have the old non-schemaless version of the CRD - if we do, bail out
		crd, err := r.mdClient.Resource(schema.GroupVersionResource{
			Group:    "apiextensions.k8s.io",
			Version:  "v1",
			Resource: "customresourcedefinitions",
		}).Get(ctx, "monitoringclusters.core.opni.io", metav1.GetOptions{})

		annotations := map[string]string{}
		if err != nil {
			if !k8serrors.IsForbidden(err) {
				// Ignore forbidden errors, because the rbac rule to allow reading CRDs
				// was added at the same time as the version of the CRD we are looking for.
				// Either way, the annotation will not be present.
				return k8sutil.DoNotRequeue(), err
			}
		}
		if crd != nil {
			annotations = crd.GetAnnotations()
		}

		if _, ok := annotations[corev1beta1.InternalSchemalessAnnotation]; !ok {
			err := fmt.Errorf("MonitoringCluster CRD is out of date, cannot continue")
			return k8sutil.DoNotRequeue(), err
		}
	}
	lg := r.logger.WithValues(
		"name", umc.GetName(),
		"namespace", umc.GetNamespace(),
	)
	if currentRevision < corev1beta1.MonitoringClusterTargetRevision {
		for i := currentRevision; i < corev1beta1.MonitoringClusterTargetRevision; i++ {
			lg.Info(fmt.Sprintf(chalk.Cyan.Color("Upgrading MonitoringCluster from revision %d to %d"), i, i+1))
			err := conversions[i](umc)
			if err != nil {
				if errors.Is(err, ErrAlreadyUpgraded) {
					lg.Info(fmt.Sprintf(chalk.Cyan.Color("MonitoringCluster is already compatible with revision %d, no changes required"), i+1))
					continue
				}
				return k8sutil.DoNotRequeue(), err
			}
		}
	} else if currentRevision > corev1beta1.MonitoringClusterTargetRevision {
		return k8sutil.DoNotRequeue(), fmt.Errorf("MonitoringCluster %s/%s was created with a newer version of Opni and is incompatible with the current version", umc.GetNamespace(), umc.GetName())
	} else {
		return k8sutil.DoNotRequeue(), nil
	}

	lg.Info(fmt.Sprintf(chalk.Green.Color("Successfully upgraded MonitoringCluster from revision %d to %d"),
		currentRevision, corev1beta1.MonitoringClusterTargetRevision))
	corev1beta1.SetMonitoringClusterRevision(umc, corev1beta1.MonitoringClusterTargetRevision)
	return k8sutil.Requeue(), nil
}

type unstructuredConverter func(*unstructured.Unstructured) error

var ErrAlreadyUpgraded = fmt.Errorf("already upgraded")

// Contains a set of functions that can be used to convert between incompatible
// revisions of the MonitoringCluster CRD.
// Each function knows how to convert from a specific version V to version
// V+1. The conversion function should modify the unsturctured object in-place.
var conversions = map[int64]unstructuredConverter{
	// 0->1: upgrade to the first revision of the schemaless CRD with generated
	// cortex config apis.
	//
	// 1. copy storage.retentionPeriod to cortexConfig.limits.compactor_blocks_retention_period
	// 2. copy storage to cortexConfig.storage with all fields changed to snake_case
	// 3. If deploymentMode is AllInOne:
	//     - set cortexWorkloads equal to:
	//         targets:
	//           all: { replicas: 1 }
	//    If deploymentMode is HighlyAvailable:
	//     - set cortexWorkloads equal to:
	//         targets:
	//           alertmanager:   { replicas: 3 }
	//           compactor:      { replicas: 3 }
	//           distributor:    { replicas: 1 }
	//           ingester:       { replicas: 3 }
	//           purger:         { replicas: 1 }
	//           querier:        { replicas: 3 }
	//           query-frontend: { replicas: 1 }
	//           ruler:          { replicas: 3 }
	//           store-gateway:  { replicas: 3 }
	// 4. for each entry in cortex.workloads:
	//     - if the replicas field is present, copy to cortexConfig.workloads.targets.[name].replicas
	//     - if the extraArgs field is present, copy to cortexConfig.workloads.targets.[name].extraArgs
	// 5. remove v0 fields

	// v0:
	// spec:
	//   cortex:
	//     enabled: bool
	//     deploymentMode: string
	//     storage:
	//       retentionPeriod:
	//         seconds: int64
	//       [rest of the fields are the same as v1]
	//   gateway:
	//		 [same as v1]
	//   grafana:
	//     [same as v1]
	//
	// v1:
	// spec:
	//   cortex:
	//     enabled: bool
	//     cortexConfig:
	//       [new fields]
	//       limits:
	//         compactor_blocks_retention_period:
	//					 seconds: int64 [move from v0 storage.retentionPeriod]
	//     cortexWorkloads:
	0: func(src *unstructured.Unstructured) error {
		content := src.UnstructuredContent()

		// check for any known v0 fields
		v0DeploymentMode, hasV0DeploymentMode, _ := unstructured.NestedString(content, "spec", "cortex", "deploymentMode")
		v0RetentionPeriod, hasV0RetentionPeriod, _ := unstructured.NestedMap(content, "spec", "cortex", "storage", "retentionPeriod")
		v0Workloads, hasV0Workloads, _ := unstructured.NestedMap(content, "spec", "cortex", "workloads")
		v0Storage, hasV0Storage, _ := unstructured.NestedMap(content, "spec", "cortex", "storage")

		if !hasV0DeploymentMode && !hasV0RetentionPeriod && !hasV0Workloads && !hasV0Storage {
			// not a v0 object
			return ErrAlreadyUpgraded
		}

		renameStorageKeys := func(in map[string]any) map[string]any {
			httpReplacements := map[string]any{
				"maxIdleConns":        "max_idle_connections",
				"maxIdleConnsPerHost": "max_idle_connections_per_host",
				"maxConnsPerHost":     "max_connections_per_host",
			}
			replacements := map[string]any{
				"s3": map[string]any{
					"http": httpReplacements,
				},
				"azure": map[string]any{
					"storageAccountName": "account_name",
					"storageAccountKey":  "account_key",
					"endpoint":           "endpoint_suffix",
					"http":               httpReplacements,
				},
				"swift": map[string]any{},
				"filesystem": map[string]any{
					"directory": "dir",
				},
			}
			return renameKeysRecursive(in, xstrings.ToSnakeCase, replacements)
		}

		v1CortexConfig := map[string]any{}
		v1CortexWorkloads := map[string]any{}
		if hasV0Storage {
			if hasV0RetentionPeriod {
				v1CortexConfig["limits"] = map[string]any{
					"compactor_blocks_retention_period": v0RetentionPeriod,
				}
				delete(v0Storage, "retentionPeriod")
			}

			v1CortexConfig["storage"] = renameStorageKeys(v0Storage)
		}
		if hasV0DeploymentMode {
			switch v0DeploymentMode {
			case "AllInOne":
				v1CortexWorkloads = map[string]any{
					"targets": map[string]any{
						"all": map[string]any{
							"replicas": int64(1),
						},
					},
				}
			case "HighlyAvailable":
				v1CortexWorkloads = map[string]any{
					"targets": map[string]any{
						"alertmanager":   map[string]any{"replicas": int64(3)},
						"compactor":      map[string]any{"replicas": int64(3)},
						"distributor":    map[string]any{"replicas": int64(1)},
						"ingester":       map[string]any{"replicas": int64(3)},
						"purger":         map[string]any{"replicas": int64(1)},
						"querier":        map[string]any{"replicas": int64(3)},
						"query-frontend": map[string]any{"replicas": int64(1)},
						"ruler":          map[string]any{"replicas": int64(3)},
						"store-gateway":  map[string]any{"replicas": int64(3)},
					},
				}
				v0ToV1TargetFields := map[string]string{
					"storeGateway":  "store-gateway",
					"queryFrontend": "query-frontend",

					"alertmanager": "alertmanager",
					"compactor":    "compactor",
					"distributor":  "distributor",
					"ingester":     "ingester",
					"purger":       "purger",
					"querier":      "querier",
					"ruler":        "ruler",
				}
				if hasV0Workloads {
					for _, v0TargetField := range lo.Keys(v0ToV1TargetFields) {
						if v0Target, ok, _ := unstructured.NestedMap(v0Workloads, v0TargetField); ok {
							if replicas, ok := v0Target["replicas"]; ok {
								unstructured.SetNestedField(v1CortexWorkloads, replicas, "targets", v0ToV1TargetFields[v0TargetField], "replicas")
							}
							if extraArgs, ok := v0Target["extraArgs"]; ok {
								unstructured.SetNestedField(v1CortexWorkloads, extraArgs, "targets", v0ToV1TargetFields[v0TargetField], "extraArgs")
							}
						}
					}
				}
			}
		}
		unstructured.RemoveNestedField(content, "spec", "cortex", "deploymentMode")
		unstructured.RemoveNestedField(content, "spec", "cortex", "storage")
		unstructured.RemoveNestedField(content, "spec", "cortex", "workloads")
		unstructured.SetNestedMap(content, v1CortexConfig, "spec", "cortex", "cortexConfig")
		unstructured.SetNestedMap(content, v1CortexWorkloads, "spec", "cortex", "cortexWorkloads")

		src.SetUnstructuredContent(content)
		return nil
	},
}

func renameKeysRecursive(in map[string]any, fn func(string) string, replacements map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		k, v := k, v
		if m, ok := v.(map[string]any); ok {
			if r, ok := replacements[k]; ok {
				v = renameKeysRecursive(m, fn, r.(map[string]any))
			} else {
				v = renameKeysRecursive(m, fn, nil)
			}
		}
		if r, ok := replacements[k]; ok {
			if str, ok := r.(string); ok {
				k = str
			}
		} else {
			k = fn(k)
		}
		out[k] = v
	}
	return out
}

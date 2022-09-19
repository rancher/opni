package cortex

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cortexVersion         string
	loadCortexVersionOnce sync.Once
)

func (r *Reconciler) updateCortexVersionStatus() (bool, error) {
	lg := r.logger
	loadCortexVersionOnce.Do(func() {
		buildInfo, ok := debug.ReadBuildInfo()
		if !ok {
			panic("could not read build info")
		}
		// https://github.com/golang/go/issues/33976
		if buildInfo.Main.Path == "" {
			cortexVersion = "(unknown)"
			return
		}
		for _, depInfo := range buildInfo.Deps {
			if depInfo.Path == "github.com/cortexproject/cortex" {
				if depInfo.Replace != nil {
					cortexVersion = depInfo.Replace.Version
				} else {
					cortexVersion = depInfo.Version
				}
				return
			}
		}
		panic("could not find cortex dependency in build info")
	})

	switch cluster := r.mc.(type) {
	case *v1beta2.MonitoringCluster:
		if cluster.Status.Cortex.Version != cortexVersion {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				cluster.Status.Cortex.Version = cortexVersion
				return r.client.Status().Update(r.ctx, cluster)
			})
			if err != nil {
				lg.Error(err, "failed to update cortex version status")
				return false, err
			}
		}
	case *corev1beta1.MonitoringCluster:
		if cluster.Status.Cortex.Version != cortexVersion {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
					return err
				}
				cluster.Status.Cortex.Version = cortexVersion
				return r.client.Status().Update(r.ctx, cluster)
			})
			if err != nil {
				lg.Error(err, "failed to update cortex version status")
				return false, err
			}
		}
	default:
		return false, errors.New("unsupported monitoring type")
	}

	return false, nil
}

func (r *Reconciler) pollCortexHealth(workloads []resources.Resource) util.RequeueOp {
	wlStatus := map[string]corev1beta1.WorkloadStatus{}
	for _, wl := range workloads {
		rtobj, desiredState, err := wl()
		if err != nil {
			return util.RequeueErr(err)
		}
		obj := rtobj.(client.Object)

		name, ok := workloadComponent(obj)
		if !ok {
			return util.RequeueErr(fmt.Errorf("workload is missing component label: %s", obj.GetName()))
		}

		switch obj.(type) {
		case *appsv1.Deployment:
			deployment := &appsv1.Deployment{}
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(obj), deployment); err != nil {
				if k8serrors.IsNotFound(err) {
					if desiredState == reconciler.StateAbsent {
						wlStatus[name] = corev1beta1.WorkloadStatus{
							Ready:   true,
							Message: "Deployment has been successfully deleted",
						}
						continue
					} else {
						wlStatus[name] = corev1beta1.WorkloadStatus{
							Ready:   false,
							Message: "Deployment has not been created yet",
						}
					}
				} else {
					return util.RequeueErr(err)
				}
			} else {
				if desiredState == reconciler.StateAbsent {
					wlStatus[name] = corev1beta1.WorkloadStatus{
						Ready:   false,
						Message: "Deployment is pending deletion",
					}
					continue
				}
			}

			if deployment.Status.ReadyReplicas == lo.FromPtrOr(deployment.Spec.Replicas, 1) {
				wlStatus[name] = corev1beta1.WorkloadStatus{
					Ready:   true,
					Message: "All replicas are ready",
				}
				continue
			} else {
				wlStatus[name] = corev1beta1.WorkloadStatus{
					Ready:   false,
					Message: "Deployment is progressing",
				}
				continue
			}
		case *appsv1.StatefulSet:
			statefulSet := &appsv1.StatefulSet{}
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(obj), statefulSet); err != nil {
				if k8serrors.IsNotFound(err) {
					if desiredState == reconciler.StateAbsent {
						wlStatus[name] = corev1beta1.WorkloadStatus{
							Ready:   true,
							Message: "StatefulSet has been successfully deleted",
						}
						continue
					} else {
						wlStatus[name] = corev1beta1.WorkloadStatus{
							Ready:   false,
							Message: "StatefulSet has not been created yet",
						}
					}
				} else {
					return util.RequeueErr(err)
				}
			} else {
				if desiredState == reconciler.StateAbsent {
					wlStatus[name] = corev1beta1.WorkloadStatus{
						Ready:   false,
						Message: "StatefulSet is pending deletion",
					}
					continue
				}
			}

			if statefulSet.Status.ReadyReplicas != lo.FromPtrOr(statefulSet.Spec.Replicas, 1) {
				wlStatus[name] = corev1beta1.WorkloadStatus{
					Ready:   false,
					Message: "StatefulSet is progressing",
				}
				continue
			}
			if statefulSet.Status.UpdateRevision != "" &&
				statefulSet.Status.UpdatedReplicas != lo.FromPtrOr(statefulSet.Spec.Replicas, 1) {
				wlStatus[name] = corev1beta1.WorkloadStatus{
					Ready:   false,
					Message: "StatefulSet is progressing",
				}
				continue
			}
			wlStatus[name] = corev1beta1.WorkloadStatus{
				Ready:   true,
				Message: "All replicas are ready",
			}
		}
	}

	conditions := []string{}
	for name, status := range wlStatus {
		if !status.Ready {
			conditions = append(conditions, fmt.Sprintf("workload %q is not ready: %s", name, status.Message))
		}
	}
	workloadsReady := len(conditions) == 0

	switch cluster := r.mc.(type) {
	case *v1beta2.MonitoringCluster:
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
				return err
			}
			cluster.Status.Cortex.WorkloadsReady = workloadsReady
			cluster.Status.Cortex.Conditions = conditions
			cluster.Status.Cortex.WorkloadStatus = make(map[string]v1beta2.WorkloadStatus)
			for k, v := range wlStatus {
				cluster.Status.Cortex.WorkloadStatus[k] = v1beta2.WorkloadStatus{
					Ready:   v.Ready,
					Message: v.Message,
				}
			}
			return r.client.Status().Update(r.ctx, cluster)
		})
		if err != nil {
			return util.RequeueErr(err)
		}
	case *corev1beta1.MonitoringCluster:
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
				return err
			}
			cluster.Status.Cortex.WorkloadsReady = workloadsReady
			cluster.Status.Cortex.Conditions = conditions
			cluster.Status.Cortex.WorkloadStatus = wlStatus
			return r.client.Status().Update(r.ctx, cluster)
		})
		if err != nil {
			return util.RequeueErr(err)
		}
	default:
		return util.RequeueErr(errors.New("unsupported monitoring type"))

	}

	if len(conditions) > 0 {
		return util.RequeueAfter(1 * time.Second)
	}
	return util.DoNotRequeue()
}

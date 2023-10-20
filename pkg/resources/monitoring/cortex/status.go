package cortex

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
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

	if r.mc.Status.Cortex.Version != cortexVersion {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.mc), r.mc); err != nil {
				return err
			}
			r.mc.Status.Cortex.Version = cortexVersion
			return r.client.Status().Update(r.ctx, r.mc)
		})
		if err != nil {
			lg.With(logger.Err(err)).Error("failed to update cortex version status")
			return false, err
		}
	}
	return false, nil
}

func (r *Reconciler) pollCortexHealth(workloads []resources.Resource) k8sutil.RequeueOp {
	wlStatus := map[string]corev1beta1.WorkloadStatus{}
	for _, wl := range workloads {
		rtobj, desiredState, err := wl()
		if err != nil {
			return k8sutil.RequeueErr(err)
		}
		obj := rtobj.(client.Object)

		name, ok := workloadComponent(obj)
		if !ok {
			return k8sutil.RequeueErr(fmt.Errorf("workload is missing component label: %s", obj.GetName()))
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
					return k8sutil.RequeueErr(err)
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
					return k8sutil.RequeueErr(err)
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

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.mc), r.mc); err != nil {
			return err
		}
		r.mc.Status.Cortex.WorkloadsReady = workloadsReady
		r.mc.Status.Cortex.Conditions = conditions
		r.mc.Status.Cortex.WorkloadStatus = wlStatus
		return r.client.Status().Update(r.ctx, r.mc)
	})
	if err != nil {
		return k8sutil.RequeueErr(err)
	}

	if len(conditions) > 0 {
		return k8sutil.RequeueAfter(1 * time.Second)
	}
	return k8sutil.DoNotRequeue()
}

package alerting_manager

import (
	"context"
	"fmt"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *AlertingGatewayManager) newOpniGateway() *corev1beta1.Gateway {
	return &corev1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.GatewayRef.Name,
			Namespace: a.GatewayRef.Namespace,
		},
	}
}

func newOpniControllerSet(ns string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName + "-internal",
			Namespace: ns,
		},
	}
}

func newOpniWorkerSet(ns string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingClusterNodeServiceName + "-internal",
			Namespace: ns,
		},
	}
}

func extractGatewayAlertingSpec(gw *corev1beta1.Gateway) *corev1beta1.AlertingSpec {
	alerting := gw.Spec.Alerting.DeepCopy()
	return alerting
}

type statusTuple lo.Tuple2[error, *appsv1.StatefulSet]

func (a *AlertingGatewayManager) alertingControllerStatus(gw *corev1beta1.Gateway) (*alertops.InstallStatus, error) {
	cs := newOpniControllerSet(a.GatewayRef.Namespace)
	ws := newOpniWorkerSet(a.GatewayRef.Namespace)

	ctrlErr := a.K8sClient.Get(context.Background(), client.ObjectKeyFromObject(cs), cs)
	workErr := a.K8sClient.Get(context.Background(), client.ObjectKeyFromObject(ws), ws)

	if gw.Spec.Alerting.Enabled {
		expectedSets := []statusTuple{{A: ctrlErr, B: cs}}
		if gw.Spec.Alerting.Replicas > 1 {
			expectedSets = append(expectedSets, statusTuple{A: workErr, B: ws})
		}
		for _, status := range expectedSets {
			if status.A != nil {
				if k8serrors.IsNotFound(status.A) {
					return &alertops.InstallStatus{
						State: alertops.InstallState_InstallUpdating,
					}, nil
				}
				return nil, fmt.Errorf("failed to get opni alerting status %w", status.A)
			}
			if status.B.Status.Replicas != status.B.Status.AvailableReplicas {
				return &alertops.InstallStatus{
					State: alertops.InstallState_InstallUpdating,
				}, nil
			}
		}
		// sanity check the desired number of replicas in the spec matches the total available replicas
		up := lo.Reduce(expectedSets, func(agg int32, status statusTuple, _ int) int32 {
			return agg + status.B.Status.AvailableReplicas
		}, 0)
		if up != gw.Spec.Alerting.Replicas {
			return &alertops.InstallStatus{
				State: alertops.InstallState_InstallUpdating,
			}, nil
		}
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	}
	if ctrlErr != nil && workErr != nil {
		if k8serrors.IsNotFound(ctrlErr) && k8serrors.IsNotFound(workErr) {
			return &alertops.InstallStatus{
				State: alertops.InstallState_NotInstalled,
			}, nil
		}
		return nil, fmt.Errorf("failed to get opni alerting controller status %w", ctrlErr)
	}

	return &alertops.InstallStatus{
		State: alertops.InstallState_Uninstalling,
	}, nil
}

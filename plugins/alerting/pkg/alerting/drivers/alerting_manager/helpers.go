package alerting_manager

import (
	"context"
	"fmt"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *AlertingManager) newOpniGateway() *corev1beta1.Gateway {
	return &corev1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.GatewayRef.Name,
			Namespace: a.GatewayRef.Namespace,
		},
	}

}

func (a *AlertingManager) newOpniControllerSet() (client.Object, error) {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName + "-internal",
			Namespace: a.GatewayRef.Namespace,
		},
	}, nil
}

func extractGatewayAlertingSpec(gw *corev1beta1.Gateway) *corev1beta1.AlertingSpec {
	alerting := gw.Spec.Alerting.DeepCopy()
	return alerting
}

func (a *AlertingManager) alertingControllerStatus(gw *corev1beta1.Gateway) (*alertops.InstallStatus, error) {
	ss, err := a.newOpniControllerSet()
	if err != nil {
		return nil, err
	}
	k8serr := a.K8sClient.Get(context.Background(), client.ObjectKeyFromObject(ss), ss)

	if gw.Spec.Alerting.Enabled {
		if k8serr != nil {
			if k8serrors.IsNotFound(k8serr) {
				return &alertops.InstallStatus{
					State: alertops.InstallState_InstallUpdating,
				}, nil
			}
			return nil, fmt.Errorf("failed to get opni alerting controller status %w", k8serr)
		}
		controller := ss.(*appsv1.StatefulSet)
		if controller.Status.Replicas != controller.Status.AvailableReplicas ||
			controller.Status.AvailableReplicas != gw.Spec.Alerting.Replicas {
			return &alertops.InstallStatus{
				State: alertops.InstallState_InstallUpdating,
			}, nil
		}
		return &alertops.InstallStatus{
			State: alertops.InstallState_Installed,
		}, nil
	}
	if k8serr != nil {
		if k8serrors.IsNotFound(k8serr) {
			return &alertops.InstallStatus{
				State: alertops.InstallState_NotInstalled,
			}, nil
		}
		return nil, fmt.Errorf("failed to get opni alerting controller status %w", k8serr)
	}
	return &alertops.InstallStatus{
		State: alertops.InstallState_Uninstalling,
	}, nil
}

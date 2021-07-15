package providers

import (
	"context"
	"errors"

	"github.com/rancher/opni/apis/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ReconcileLogAdapter(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	switch logAdapter.Spec.Provider {
	case v1beta1.LogProviderAKS:
		return reconcileAKS(ctx, cli, logAdapter)
	case v1beta1.LogProviderEKS:
		return reconcileEKS(ctx, cli, logAdapter)
	case v1beta1.LogProviderGKE:
		return reconcileGKE(ctx, cli, logAdapter)
	case v1beta1.LogProviderK3S:
		return reconcileK3S(ctx, cli, logAdapter)
	case v1beta1.LogProviderRKE:
		return reconcileRKE(ctx, cli, logAdapter)
	case v1beta1.LogProviderRKE2:
		return reconcileRKE2(ctx, cli, logAdapter)
	case v1beta1.LogProviderKubeAudit:
		return reconcileKubeAudit(ctx, cli, logAdapter)
	}

	return ctrl.Result{}, errors.New("unsupported provider")
}

func reconcileAKS(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileEKS(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileGKE(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileK3S(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileRKE(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileRKE2(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func reconcileKubeAudit(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

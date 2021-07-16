package providers

import (
	"context"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta1"
	opnierrors "github.com/rancher/opni/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func ReconcileLogAdapter(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	switch logAdapter.Spec.Provider {
	case v1beta1.LogProviderAKS, v1beta1.LogProviderEKS, v1beta1.LogProviderGKE:
		return reconcileGenericCloud(ctx, cli, logAdapter)
	case v1beta1.LogProviderK3S:
		return reconcileK3S(ctx, cli, logAdapter)
	case v1beta1.LogProviderRKE:
		return reconcileRKE(ctx, cli, logAdapter)
	case v1beta1.LogProviderRKE2:
		return reconcileRKE2(ctx, cli, logAdapter)
	case v1beta1.LogProviderKubeAudit:
		return reconcileKubeAudit(ctx, cli, logAdapter)
	}

	return ctrl.Result{},
		fmt.Errorf("%w: %s", opnierrors.UnsupportedProvider, logAdapter.Spec.Provider)
}

func reconcileGenericCloud(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	rec := reconciler.NewReconcilerWith(cli,
		reconciler.WithLog(lg),
		reconciler.WithScheme(cli.Scheme()),
	)

	logging := BuildLogging(logAdapter)
	result := reconciler.CombinedResult{}
	result.Combine(rec.ReconcileResource(logging, reconciler.StatePresent))
	return result.Result, result.Err
}

func reconcileK3S(
	ctx context.Context,
	cli client.Client,
	logAdapter *v1beta1.LogAdapter,
) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	rec := reconciler.NewReconcilerWith(cli,
		reconciler.WithLog(lg),
		reconciler.WithScheme(cli.Scheme()),
	)

	logging := BuildLogging(logAdapter)
	config := BuildK3SConfig(logAdapter)
	aggregator := BuildK3SJournaldAggregator(logAdapter)
	svcAcct := BuildK3SServiceAccount(logAdapter)

	switch logAdapter.Spec.K3S.ContainerEngine {
	case v1beta1.ContainerEngineSystemd:
		result := reconciler.CombinedResult{}
		result.Combine(rec.ReconcileResource(logging, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(config, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(aggregator, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(svcAcct, reconciler.StatePresent))
		return result.Result, result.Err
	case v1beta1.ContainerEngineOpenRC:
		result := reconciler.CombinedResult{}
		result.Combine(rec.ReconcileResource(logging, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(config, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(aggregator, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(svcAcct, reconciler.StateAbsent))
		return result.Result, result.Err
	}

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

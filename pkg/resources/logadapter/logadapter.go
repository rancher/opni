package logadapter

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func ReconcileLogAdapter(
	ctx context.Context,
	cli client.Client,
	logAdapter *opniloggingv1beta1.LogAdapter,
) (ctrl.Result, error) {
	result := reconciler.CombinedResult{}

	lg := log.FromContext(ctx)

	rec := reconciler.NewReconcilerWith(cli,
		reconciler.WithLog(lg),
		reconciler.WithScheme(cli.Scheme()),
	)

	reconcileRootLogging(rec, logAdapter, &result)

	switch logAdapter.Spec.Provider {
	case opniloggingv1beta1.LogProviderAKS, opniloggingv1beta1.LogProviderEKS, opniloggingv1beta1.LogProviderGKE:
		reconcileGenericCloud(rec, logAdapter, &result)
	case opniloggingv1beta1.LogProviderK3S:
		reconcileK3S(rec, logAdapter, &result)
	case opniloggingv1beta1.LogProviderRKE:
		reconcileRKE(rec, logAdapter, &result)
	case opniloggingv1beta1.LogProviderRKE2:
		reconcileRKE2(rec, logAdapter, &result)
	}

	return result.Result, result.Err
}

func reconcileRootLogging(rec reconciler.ResourceReconciler,
	instance *opniloggingv1beta1.LogAdapter,
	result *reconciler.CombinedResult,
) {
	rootLogging := BuildRootLogging(instance)
	result.Combine(rec.ReconcileResource(rootLogging, reconciler.StatePresent))
}

func reconcileGenericCloud(
	rec reconciler.ResourceReconciler,
	instance *opniloggingv1beta1.LogAdapter,
	result *reconciler.CombinedResult,
) {
	logging := BuildLogging(instance)
	result.Combine(rec.ReconcileResource(logging, reconciler.StatePresent))
}

func reconcileK3S(
	rec reconciler.ResourceReconciler,
	logAdapter *opniloggingv1beta1.LogAdapter,
	result *reconciler.CombinedResult,
) {
	logging := BuildLogging(logAdapter)
	config := BuildK3SConfig(logAdapter)
	aggregator := BuildK3SJournaldAggregator(logAdapter)
	svcAcct := BuildK3SServiceAccount(logAdapter)

	switch logAdapter.Spec.K3S.ContainerEngine {
	case opniloggingv1beta1.ContainerEngineSystemd:
		result.Combine(rec.ReconcileResource(logging, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(config, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(aggregator, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(svcAcct, reconciler.StatePresent))
	case opniloggingv1beta1.ContainerEngineOpenRC:
		result.Combine(rec.ReconcileResource(logging, reconciler.StatePresent))
		result.Combine(rec.ReconcileResource(config, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(aggregator, reconciler.StateAbsent))
		result.Combine(rec.ReconcileResource(svcAcct, reconciler.StateAbsent))
	}
}

func reconcileRKE(
	rec reconciler.ResourceReconciler,
	instance *opniloggingv1beta1.LogAdapter,
	result *reconciler.CombinedResult,
) {
	config := BuildRKEConfig(instance)
	aggregator := BuildRKEAggregator(instance)
	svcAcct := BuildRKEServiceAccount(instance)

	result.Combine(rec.ReconcileResource(config, reconciler.StatePresent))
	result.Combine(rec.ReconcileResource(aggregator, reconciler.StatePresent))
	result.Combine(rec.ReconcileResource(svcAcct, reconciler.StatePresent))
}

func reconcileRKE2(
	rec reconciler.ResourceReconciler,
	instance *opniloggingv1beta1.LogAdapter,
	result *reconciler.CombinedResult,
) {
	config := BuildRKE2Config(instance)
	aggregator := BuildRKE2JournaldAggregator(instance)
	svcAcct := BuildRKE2ServiceAccount(instance)

	result.Combine(rec.ReconcileResource(config, reconciler.StatePresent))
	result.Combine(rec.ReconcileResource(aggregator, reconciler.StatePresent))
	result.Combine(rec.ReconcileResource(svcAcct, reconciler.StatePresent))
}

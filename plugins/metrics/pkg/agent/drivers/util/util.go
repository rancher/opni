package util

import (
	"context"
	"errors"
	"sync"

	"github.com/cisco-open/k8s-objectmatcher/patch"
	"github.com/rancher/opni/pkg/logger"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ReconcilerState struct {
	sync.Mutex
	running       bool
	backoffCtx    context.Context
	backoffCancel context.CancelFunc
}

func (r *ReconcilerState) GetRunning() bool {
	r.Lock()
	defer r.Unlock()
	return r.running
}

func (r *ReconcilerState) SetRunning(running bool) {
	r.Lock()
	defer r.Unlock()
	r.running = running
}

func (r *ReconcilerState) Cancel() {
	r.Lock()
	defer r.Unlock()
	if r.backoffCancel != nil {
		r.backoffCancel()
	}
}

func (r *ReconcilerState) SetBackoffCtx(ctx context.Context, cancel context.CancelFunc) {
	r.Lock()
	defer r.Unlock()
	r.backoffCtx = ctx
	r.backoffCancel = cancel
}

// (obj, shouldExist))
type ReconcileItem lo.Tuple2[client.Object, bool]

func ReconcileObject(ctx context.Context, k8sClient client.Client, namespace string, item ReconcileItem) error {
	desired, shouldExist := item.A, item.B
	// get the object
	key := client.ObjectKeyFromObject(desired)
	lg := logger.PluginLoggerFromContext(ctx).With("object", key)
	lg.Info("reconciling object")

	// get the agent statefulset
	list := &appsv1.DeploymentList{}
	if err := k8sClient.List(context.TODO(), list,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"opni.io/app": "agent",
		},
	); err != nil {
		return err
	}

	if len(list.Items) != 1 {
		return errors.New("deployments found not exactly 1")
	}
	agentDep := &list.Items[0]

	current := desired.DeepCopyObject().(client.Object)
	err := k8sClient.Get(context.TODO(), key, current)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	// this can error if the object is cluster-scoped, but that's ok
	controllerutil.SetOwnerReference(agentDep, desired, k8sClient.Scheme())

	if k8serrors.IsNotFound(err) {
		if !shouldExist {
			lg.Info("object does not exist and should not exist, skipping")
			return nil
		}
		lg.Info("object does not exist, creating")
		// create the object
		return k8sClient.Create(context.TODO(), desired)
	} else if !shouldExist {
		// delete the object
		lg.Info("object exists and should not exist, deleting")
		return k8sClient.Delete(context.TODO(), current)
	}

	// update the object
	patchResult, err := patch.DefaultPatchMaker.Calculate(current, desired, patch.IgnoreStatusFields())
	if err != nil {
		lg.With(
			logger.Err(err),
		).Warn("could not match objects")

		return err
	}
	if patchResult.IsEmpty() {
		lg.Info("resource is in sync")
		return nil
	}
	lg.Info("resource diff")

	if err := patch.DefaultAnnotator.SetLastAppliedAnnotation(desired); err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to set last applied annotation")
	}

	metaAccessor := meta.NewAccessor()

	currentResourceVersion, err := metaAccessor.ResourceVersion(current)
	if err != nil {
		return err
	}
	if err := metaAccessor.SetResourceVersion(desired, currentResourceVersion); err != nil {
		return err
	}

	lg.Info("updating resource")

	return k8sClient.Update(context.TODO(), desired)
}

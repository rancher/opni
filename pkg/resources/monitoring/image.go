package monitoring

import (
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) updateImageStatus() (bool, error) {
	lg := r.logger

	image, pullPolicy, err := resources.FindManagerImage(r.ctx, r.client)
	if err != nil {
		return false, err
	}

	if r.mc.Status.Image != image {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.mc), r.mc); err != nil {
				return err
			}
			r.mc.Status.Image = image
			r.mc.Status.ImagePullPolicy = pullPolicy
			return r.client.Status().Update(r.ctx, r.mc)
		})
		if err != nil {
			lg.Error("failed to update monitoring cluster status", logger.Err(err))
			return false, err
		}
		return true, nil
	}
	return false, nil
}

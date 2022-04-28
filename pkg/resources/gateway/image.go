package gateway

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
)

func (r *Reconciler) updateImageStatus() (bool, error) {
	lg := r.logger
	var image string
	var pullPolicy corev1.PullPolicy
	if imgOverride := r.gw.Spec.Image.GetImageWithDefault(""); imgOverride != "" {
		image = imgOverride
	} else {
		var err error
		image, pullPolicy, err = resources.FindManagerImage(r.ctx, r.client)
		if err != nil {
			return false, err
		}
	}

	if r.gw.Status.Image != image {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			r.gw.Status.Image = image
			r.gw.Status.ImagePullPolicy = pullPolicy
			return r.client.Status().Update(r.ctx, r.gw)
		})
		if err != nil {
			lg.Error(err, "failed to update monitoring cluster status")
			return false, err
		}
		return true, nil
	}
	return false, nil
}

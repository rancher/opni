package monitoring

import (
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) updateImageStatus() (bool, error) {
	lg := r.logger
	var image string
	var pullPolicy corev1.PullPolicy
	if imgOverride := r.mc.Spec.Image.GetImageWithDefault(""); imgOverride != "" {
		image = imgOverride
	} else {
		deploymentNamespace := os.Getenv("OPNI_SYSTEM_NAMESPACE")
		if deploymentNamespace == "" {
			panic("missing downward API env vars")
		}
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-controller-manager",
				Namespace: deploymentNamespace,
			},
		}
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(deployment), deployment); err != nil {
			return false, err
		}
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == "manager" {
				image = container.Image
				pullPolicy = container.ImagePullPolicy
			}
		}

		if image == "" {
			panic(fmt.Sprintf("manager container not found in deployment %s/opni-controller-manager", deploymentNamespace))
		}
	}

	if r.mc.Status.Image != image {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			r.mc.Status.Image = image
			r.mc.Status.ImagePullPolicy = pullPolicy
			return r.client.Status().Update(r.ctx, r.mc)
		})
		if err != nil {
			lg.Error(err, "failed to update monitoring cluster status")
			return false, err
		}
		return true, nil
	}
	return false, nil
}

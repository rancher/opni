package v1beta1

import (
	"github.com/rancher/opni/apis/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *PretrainedModel) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.PretrainedModel)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Hyperparameters = src.Spec.Hyperparameters

	if src.Spec.HTTP != nil {
		http := &v1beta2.HTTPSource{
			URL: src.Spec.HTTP.URL,
		}
		dst.Spec.HTTP = http
	}

	if src.Spec.Container != nil {
		container := &v1beta2.ContainerSource{
			Image:            src.Spec.Container.Image,
			ImagePullSecrets: src.Spec.Container.ImagePullSecrets,
		}
		dst.Spec.Container = container
	}

	return nil
}

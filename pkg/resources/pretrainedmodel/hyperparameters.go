package pretrainedmodel

import (
	"encoding/json"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) hyperparameters() (runtime.Object, reconciler.DesiredState, error) {
	data, err := json.MarshalIndent(r.model.Spec.Hyperparameters, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.model.Name + "-hyperparameters",
			Namespace: r.model.Namespace,
			Labels: map[string]string{
				resources.PartOfLabel:          "opni",
				resources.PretrainedModelLabel: r.model.Name,
			},
		},
		Data: map[string]string{
			"hyperparameters.json": string(data),
		},
	}
	ctrl.SetControllerReference(r.model, cm, r.client.Scheme())
	return cm, reconciler.StatePresent, nil
}

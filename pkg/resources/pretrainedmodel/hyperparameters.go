package pretrainedmodel

import (
	"encoding/json"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (r *Reconciler) hyperparameters() (runtime.Object, reconciler.DesiredState, error) {
	data, err := json.MarshalIndent(r.model.Spec.Hyperparameters, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("opni-%s-hyperparameters", r.model.Name),
			Namespace: r.model.Namespace,
			Labels: map[string]string{
				resources.PartOfLabel:          "opni",
				resources.PretrainedModelLabel: r.model.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: r.model.APIVersion,
					Kind:       r.model.Kind,
					Name:       r.model.Name,
					UID:        r.model.UID,
				},
			},
		},
		Data: map[string]string{
			"hyperparameters.json": string(data),
		},
	}
	return cm, reconciler.StatePresent, nil
}

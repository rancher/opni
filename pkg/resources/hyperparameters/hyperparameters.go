package hyperparameters

import (
	"encoding/json"
	"fmt"

	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GenerateHyperparametersConfigMap(modelName string, namespace string, hyperparameters map[string]intstr.IntOrString) (corev1.ConfigMap, error) {
	data, err := json.MarshalIndent(hyperparameters, "", "  ")
	if err != nil {
		return corev1.ConfigMap{}, err
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-hyperparameters", modelName),
			Namespace: namespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string]string{
			"hyperparameters.json": string(data),
		},
	}
	return cm, nil
}

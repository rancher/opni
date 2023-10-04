package controllers_test

import (
	"context"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// FIXME: https://github.com/rancher/opni/issues/1742
var _ = XDescribe("AI PretrainedModel Controller", Ordered, Label("controller"), func() {
	It("should reconcile pretrained model resources", func() {
		By("Creating a pretrainedmodel")
		model := &aiv1beta1.PretrainedModel{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: makeTestNamespace(),
			},
			Spec: aiv1beta1.PretrainedModelSpec{
				ModelSource: aiv1beta1.ModelSource{
					HTTP: &aiv1beta1.HTTPSource{
						URL: "https://nonexistent",
					},
				},
				Hyperparameters: map[string]intstr.IntOrString{
					"batch_size": intstr.FromInt(32),
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), model)).To(Succeed())

		By("checking if a configmap was created")
		hpConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-test-hyperparameters",
				Namespace: model.Namespace,
			},
		}
		Eventually(Object(hpConfigMap)).Should(ExistAnd(
			HaveOwner(model),
			HaveData("hyperparameters.json",
				marshal(map[string]intstr.IntOrString{
					"batch_size": intstr.FromInt(32),
				}),
			)),
		)

		newParameters := map[string]intstr.IntOrString{
			"batch_size": intstr.FromInt(32),
		}
		By("updating the hyperparameters")
		updateObject(model, func(obj *aiv1beta1.PretrainedModel) {
			obj.Spec.Hyperparameters = newParameters
		})

		By("checking if the configmap was updated")
		Eventually(Object(hpConfigMap)).Should(ExistAnd(
			HaveData("hyperparameters.json", marshal(newParameters)),
		))

		By("manually modifying the configmap")
		updateObject(hpConfigMap, func(obj *corev1.ConfigMap) {
			obj.Data["hyperparameters.json"] = `{"foo": "bar"}`
		})

		By("checking if the configmap was reverted")
		Eventually(Object(hpConfigMap)).Should(ExistAnd(
			HaveData("hyperparameters.json", marshal(newParameters)),
		))
		Consistently(Object(hpConfigMap)).Should(ExistAnd(
			HaveData("hyperparameters.json", marshal(newParameters)),
		))

		By("deleting the pretrainedmodel")
		Expect(k8sClient.Delete(context.Background(), model)).To(Succeed())

		By("checking if the configmap was deleted")
		Eventually(Object(hpConfigMap)).ShouldNot(Exist())
	})
})

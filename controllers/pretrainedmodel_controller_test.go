package controllers

import (
	"bytes"
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// random test data
var hyperparameters = []v1beta1.Hyperparameter{
	{
		Name:  "learning_rate",
		Value: "0.01",
	},
	{
		Name:  "batch_size",
		Value: "32",
	},
	{
		Name:  "epochs",
		Value: "10",
	},
	{
		Name:  "optimizer",
		Value: "adam",
	},
	{
		Name:  "loss",
		Value: "categorical_crossentropy",
	},
}

var _ = Describe("PretrainedModel Controller", func() {
	model := v1beta1.PretrainedModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: v1beta1.PretrainedModelSpec{
			ModelSource: v1beta1.ModelSource{
				HTTP: &v1beta1.HTTPSource{
					URL: "https://nonexistent",
				},
			},
			Hyperparameters: hyperparameters,
		},
	}
	When("creating a pretrainedmodel", func() {
		It("should succeed", func() {
			err := k8sClient.Create(context.Background(), &model)
			Expect(err).NotTo(HaveOccurred())
		})
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hyperparameters",
				Namespace: "default",
			},
		}
		It("should create a configmap", func() {
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
				if err != nil {
					return false
				}
				return true
			}).Should(BeTrue())
		})
		It("should contain the correct json data", func() {
			data := cm.Data["hyperparameters.json"]
			// Compact the json data for comparison
			buf := new(bytes.Buffer)
			err := json.Compact(buf, []byte(data))
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(`[{"name":"learning_rate","value":"0.01"},{"name":"batch_size","value":"32"},{"name":"epochs","value":"10"},{"name":"optimizer","value":"adam"},{"name":"loss","value":"categorical_crossentropy"}]`))
		})
	})
	When("updating the hyperparameters", func() {
		It("should succeed", func() {
			model.Spec.Hyperparameters[0].Value = "0.001"
			err := k8sClient.Update(context.Background(), &model)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should update the configmap with the new data", func() {
			Eventually(func() bool {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hyperparameters",
						Namespace: "default",
					},
				}
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
				if err != nil {
					return false
				}
				data := cm.Data["hyperparameters.json"]
				// Compact the json data for comparison
				buf := new(bytes.Buffer)
				err = json.Compact(buf, []byte(data))
				Expect(err).NotTo(HaveOccurred())
				return buf.String() == `[{"name":"learning_rate","value":"0.001"},{"name":"batch_size","value":"32"},{"name":"epochs","value":"10"},{"name":"optimizer","value":"adam"},{"name":"loss","value":"categorical_crossentropy"}]`
			}).Should(BeTrue())
		})
	})
	When("the configmap is manually modified", func() {
		It("should restore the contents of the configmap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hyperparameters",
					Namespace: "default",
				},
			}
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
			Expect(err).NotTo(HaveOccurred())
			// Modify the json data
			data := `[{"name":"learning_rate","value":"0.001"},{"name":"batch_size","value":"32"},{"name":"epochs","value":"10"},{"name":"optimizer","value":"adam"},{"name":"loss","value":"categorical_crossentropy"},{"name":"extra","value":"value"}]`
			cm.Data["hyperparameters.json"] = data
			err = k8sClient.Update(context.Background(), cm)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
				if err != nil {
					return false
				}
				data := cm.Data["hyperparameters.json"]
				// Compact the json data for comparison
				buf := new(bytes.Buffer)
				err = json.Compact(buf, []byte(data))
				Expect(err).NotTo(HaveOccurred())
				return buf.String() == `[{"name":"learning_rate","value":"0.001"},{"name":"batch_size","value":"32"},{"name":"epochs","value":"10"},{"name":"optimizer","value":"adam"},{"name":"loss","value":"categorical_crossentropy"}]`
			})
		})
	})
	When("deleting a pretrainedmodel", func() {
		It("should succeed", func() {
			err := k8sClient.Delete(context.Background(), &model)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should delete the configmap", func() {
			Eventually(func() bool {
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hyperparameters",
						Namespace: "default",
					},
				}
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
				if err != nil {
					return false
				}
				return true
			})
		})
	})
})

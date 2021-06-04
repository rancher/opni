package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rancher/opni/api/v1beta1"
)

const (
	crName      = "test-opnicluster"
	crNamespace = "opnicluster-test"
	timeout     = 10 * time.Second
	interval    = 500 * time.Millisecond
)

var _ = Describe("OpniCluster Controller", func() {
	When("creating an opnicluster ", func() {
		cluster := v1beta1.OpniCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.GroupVersion.String(),
				Kind:       "OpniCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: crNamespace,
			},
			Spec: v1beta1.OpniClusterSpec{
				Elastic: v1beta1.ElasticSpec{
					Credentials: v1beta1.CredentialsSpec{
						Keys: &v1beta1.KeysSpec{
							AccessKey: "testAccessKey",
							SecretKey: "testSecretKey",
						},
					},
				},
			},
		}
		It("should succeed", func() {
			Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
			Eventually(func() error {
				cluster := v1beta1.OpniCluster{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      crName,
					Namespace: crNamespace,
				}, &cluster)
			}, timeout, interval).Should(BeNil())
		})
		It("should create a secret with user-specified keys", func() {
			Eventually(func() string {
				secret := corev1.Secret{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      "es-config",
					Namespace: crNamespace,
				}, &secret)
				if err != nil {
					return err.Error()
				}
				if string(secret.Data["username"]) == "testAccessKey" &&
					string(secret.Data["password"]) == "testSecretKey" {
					return ""
				}
				return "keys do not match"
			}, timeout, interval).Should(BeEmpty())
		})
	})
})

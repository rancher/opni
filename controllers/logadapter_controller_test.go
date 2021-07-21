package controllers

import (
	"context"
	"fmt"
	"strings"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rancher/opni/apis/v1beta1"
)

var _ = FDescribe("LogAdapter Controller", func() {
	var (
		logadapter v1beta1.LogAdapter
		cluster    v1beta1.OpniCluster
		err        error
	)
	When("creating a logadapter", func() {
		AfterEach(func() {
			k8sClient.Delete(context.Background(), &logadapter)
		})
		BeforeEach(func() {
			logadapter = v1beta1.LogAdapter{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.GroupVersion.String(),
					Kind:       "LogAdapter",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      laName,
					Namespace: laNamespace,
				},
				Spec: v1beta1.LogAdapterSpec{
					OpniCluster: v1beta1.OpniClusterNameSpec{
						Name:      crName,
						Namespace: laNamespace,
					},
				},
			}
			cluster = v1beta1.OpniCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.GroupVersion.String(),
					Kind:       "OpniCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: laNamespace,
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
		})
		JustBeforeEach(func() {
			logadapter.Default()
			k8sClient.Create(context.Background(), &cluster)
			err = k8sClient.Create(context.Background(), &logadapter)
		})
		Context("with rke provider", func() {
			BeforeEach(func() {
				logadapter.Spec.Provider = v1beta1.LogProviderRKE
			})
			When("the spec is valid", func() {
				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() error {
						logadapter := v1beta1.LogAdapter{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      laName,
							Namespace: laNamespace,
						}, &logadapter)
					}, timeout, interval).Should(BeNil())
				})
				It("should create a logging", func() {
					Eventually(func() error {
						logging := loggingv1beta1.Logging{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      laName,
							Namespace: laNamespace,
						}, &logging)
					}, timeout, interval).Should(BeNil())
				})
				It("should create a daemonset", func() {
					Eventually(func() error {
						ds := appsv1.DaemonSet{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}, timeout, interval).Should(BeNil())
				})
			})
			When("a log level is not specified", func() {
				It("should create a configmap with default log level", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						if strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelInfo)) {
							return ""
						}
						return "config map has incorrect log level"
					})
				})
			})
			When("warning log level is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE = &v1beta1.RKESpec{
						LogLevel: v1beta1.LogLevelWarn,
					}
				})
				It("should create a config map with warn log level", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						if strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelWarn)) {
							return ""
						}
						return "config map has incorrect log level"
					})
				})
			})
			When("the OpniCluster does not exist", func() {
				BeforeEach(func() {
					logadapter.Spec.OpniCluster.Name = "doesnotexist"
				})
				It("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() error {
						logadapter := v1beta1.LogAdapter{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      laName,
							Namespace: laNamespace,
						}, &logadapter)
					}, timeout, interval).Should(BeNil())
				})
				It("should not create a logging", func() {
					Eventually(func() error {
						logging := loggingv1beta1.Logging{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      laName,
							Namespace: laNamespace,
						}, &logging)
					}, timeout, interval).ShouldNot(BeNil())
				})
				It("should not create a daemonset", func() {
					Eventually(func() error {
						ds := appsv1.DaemonSet{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}, timeout, interval).ShouldNot(BeNil())
				})
				It("should not create a configmap", func() {
					Eventually(func() error {
						configmap := corev1.ConfigMap{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
						}, &configmap)
					}, timeout, interval).ShouldNot(BeNil())
				})
			})
		})
	})
})

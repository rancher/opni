package demo

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rancher/opni/apis/demo/v1alpha1"
)

const (
	crName      = "test-opnidemo"
	crNamespace = "opnidemo-test"
	timeout     = 10 * time.Second
	interval    = 500 * time.Millisecond
)

var _ = Describe("OpniDemo Controller", func() {
	When("creating an opnidemo", func() {
		demo := v1alpha1.OpniDemo{
			ObjectMeta: v1.ObjectMeta{
				Name:      crName,
				Namespace: crNamespace,
			},
			Spec: v1alpha1.OpniDemoSpec{
				Components: v1alpha1.ComponentsSpec{
					Infra: v1alpha1.InfraStack{
						DeployHelmController: true,
						DeployNvidiaPlugin:   true,
					},
					Opni: v1alpha1.OpniStack{
						Minio: v1alpha1.ChartOptions{
							Enabled: true,
						},
						Nats: v1alpha1.ChartOptions{
							Enabled: true,
						},
						Elastic: v1alpha1.ChartOptions{
							Enabled: true,
						},
						RancherLogging: v1alpha1.ChartOptions{
							Enabled: true,
						},
						DeployGpuServices: true,
					},
				},
				MinioAccessKey:         "testAccessKey",
				MinioSecretKey:         "testSecretKey",
				MinioVersion:           "1",
				NatsVersion:            "1",
				NatsPassword:           "password",
				NatsReplicas:           1,
				NatsMaxPayload:         12345,
				NvidiaVersion:          "1",
				ElasticsearchUser:      "user",
				ElasticsearchPassword:  "password",
				NulogServiceCPURequest: "1",
				NulogTrainImage:        "does-not-exist/name:tag",
			},
		}
		It("should succeed", func() {
			Expect(k8sClient.Create(context.Background(), &demo)).To(Succeed())
			Eventually(func() error {
				cluster := v1alpha1.OpniDemo{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      crName,
					Namespace: crNamespace,
				}, &cluster)
			}, timeout, interval).Should(BeNil())
		})
		It("should create helm charts", func() {
			wg := &sync.WaitGroup{}
			for _, chart := range []string{
				"minio",
				"nats",
				"opendistro-es",
				"rancher-logging-crd",
				"rancher-logging",
			} {
				wg.Add(1)
				go func(chart string) {
					defer GinkgoRecover()
					Eventually(func() error {
						helmchart := &helmv1.HelmChart{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Namespace: crNamespace,
							Name:      chart,
						}, helmchart)
						if err != nil {
							log.Println(err)
						}
						return err
					}, timeout, interval).Should(BeNil())
					wg.Done()
				}(chart)
			}
			wg.Wait()
		})
		It("should install the helm controller", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "helm-controller",
				}, deployment)
			}, timeout, interval)
			Eventually(func() error {
				role := &rbacv1.ClusterRole{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "helm-controller",
				}, role)
				if err != nil {
					return err
				}
				if role.Rules[0].APIGroups[0] != "*" ||
					role.Rules[0].Resources[0] != "*" ||
					role.Rules[0].Verbs[0] != "*" {
					return errors.New("invalid helm controller permissions")
				}
				return nil
			}, timeout, interval)
		})
		It("should create drain service", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "drain-service",
				}, deployment)
			}, timeout, interval)
		})
		It("should create nulog inference service (control plane)", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "nulog-inference-service-control-plane",
				}, deployment)
			}, timeout, interval)
		})
		It("should create nulog inference service", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "nulog-inference-service",
				}, deployment)
			}, timeout, interval)
		})
		It("should install the nvidia plugins", func() {
			Eventually(func() error {
				deployment := &appsv1.DaemonSet{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "nvidia-device-plugin-ds",
				}, deployment)
			}, timeout, interval)
		})
		It("should create the payload receiver service", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "payload-receiver-service",
				}, deployment)
			}, timeout, interval)
			Eventually(func() error {
				svc := &corev1.Service{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "payload-receiver-service",
				}, svc)
			}, timeout, interval)
		})
		It("should create the preprocessing service", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "preprocessing-service",
				}, deployment)
			}, timeout, interval)
		})
		It("should create the training controller", func() {
			Eventually(func() error {
				deployment := &appsv1.Deployment{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "training-controller",
				}, deployment)
				if err != nil {
					return err
				}
				env := deployment.Spec.Template.Spec.Containers[0].Env
				var foundName, foundTag bool
				for _, v := range env {
					if v.Name == "NULOG_TRAIN_IMAGE_NAME" && strings.HasSuffix(v.Value, "does-not-exist/name") {
						foundName = true
					} else if v.Name == "NULOG_TRAIN_IMAGE_TAG" && v.Value == "tag" {
						foundTag = true
					}
				}
				if !foundName || !foundTag {
					return errors.New("NULOG_TRAIN_IMAGE_* env vars do not match")
				}
				return nil
			}, timeout, interval)
			Eventually(func() error {
				acct := &corev1.ServiceAccount{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "training-controller-rb",
				}, acct)
			}, timeout, interval)
			Eventually(func() error {
				acct := &rbacv1.ClusterRoleBinding{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "training-controller-rb",
				}, acct)
			}, timeout, interval)
			Eventually(func() error {
				acct := &rbacv1.ClusterRole{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "training-controller-rb",
				}, acct)
			}, timeout, interval)
		})
		It("should create the kibana dashboards pod", func() {
			Eventually(func() error {
				pod := &corev1.Pod{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "deploy-opni-kibana-dasbhboards",
				}, pod)
			}, timeout, interval)
		})
		It("should create the logging CRs", func() {
			Eventually(func() error {
				clusterFlow := loggingv1beta1.ClusterFlow{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "aiops-demo-log-flow",
				}, &clusterFlow)
			}, timeout, interval)
			Eventually(func() error {
				clusterOutput := loggingv1beta1.ClusterOutput{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Namespace: crNamespace,
					Name:      "aiops-demo-log-output",
				}, &clusterOutput)
			}, timeout, interval)
		})
	})
})

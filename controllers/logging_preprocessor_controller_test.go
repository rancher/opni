package controllers

import (
	"context"
	"fmt"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	opsterv1 "opensearch.opster.io/api/v1"
)

var _ = Describe("Logging Preprocessor Controller", Ordered, Label("controller"), func() {
	var (
		ns       string
		instance *opniloggingv1beta1.Preprocessor
	)

	When("creating a preprocessor resource", func() {
		It("should succeed", func() {
			ns = makeTestNamespace()
			certMgr.PopulateK8sObjects(context.Background(), k8sClient, ns)
			opensearch := &opsterv1.OpenSearchCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: ns,
				},
				Spec: opsterv1.ClusterSpec{
					General: opsterv1.GeneralConfig{
						Version: "1.0.0",
					},
					NodePools: []opsterv1.NodePool{
						{
							Component: "test",
							Roles: []string{
								"data",
								"master",
							},
						},
					},
				},
			}
			err := k8sClient.Create(context.Background(), opensearch)
			Expect(err).NotTo(HaveOccurred())
			Eventually(Object(opensearch)).Should(Exist())

			instance = &opniloggingv1beta1.Preprocessor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: ns,
				},
				Spec: opniloggingv1beta1.PreprocessorSpec{
					WriteIndex: "test-index",
					OpensearchCluster: &opnimeta.OpensearchClusterRef{
						Name: opensearch.Name,
					},
				},
			}
			err = k8sClient.Create(context.Background(), instance)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should create a configmap", func() {
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-preprocess-config", instance.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("config.yaml", nil),
				HaveOwner(instance),
			))
		})
		It("should create a deployment", func() {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-otel-preprocessor", instance.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-preprocessor"),
					HaveVolumeMounts("preprocessor-config"),
					HaveVolumeMounts("certs"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("preprocessor-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("Secret"),
					HaveName("certs"),
				)),
				HaveOwner(instance),
			))
		})
		It("should create a service", func() {
			Eventually(Object(&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-preprocess-svc", instance.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveOwner(instance),
				HavePorts(4317),
			))
		})
	})
})

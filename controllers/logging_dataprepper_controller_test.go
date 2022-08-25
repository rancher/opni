package controllers

import (
	"context"
	"fmt"
	"strings"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Logging DataPrepper Controller", Ordered, Label("controller"), func() {
	var (
		testNs         string
		dataPrepper    *loggingv1beta1.DataPrepper
		passwordSecret *corev1.Secret
		labels         map[string]string
	)
	Specify("setup", func() {
		testNs = makeTestNamespace()

		passwordSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "password-secret",
				Namespace: testNs,
			},
			StringData: map[string]string{
				"password": "test-password",
			},
		}
		err := k8sClient.Create(context.Background(), passwordSecret)
		Expect(err).NotTo(HaveOccurred())

		dataPrepper = &loggingv1beta1.DataPrepper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dataprepper",
				Namespace: testNs,
			},
			Spec: loggingv1beta1.DataPrepperSpec{
				Version:   "latest",
				ClusterID: "dummyid",
				Username:  "test-username",
				Opensearch: &loggingv1beta1.OpensearchSpec{
					Endpoint: "https://opensearch.example.com",
				},
				PasswordFrom: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: passwordSecret.Name,
					},
					Key: "password",
				},
			},
		}
		err = k8sClient.Create(context.Background(), dataPrepper)
		Expect(err).NotTo(HaveOccurred())

		labels = map[string]string{
			resources.AppNameLabel:  "dataprepper",
			resources.PartOfLabel:   "opni",
			resources.OpniClusterID: "dummyid",
		}
	})
	When("a log adapter is created", func() {
		It("should create a config secret", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-config", dataPrepper.Name),
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(dataPrepper),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, fmt.Sprintf(`hosts: ["%s"]`, dataPrepper.Spec.Opensearch.Endpoint))
				}),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, fmt.Sprintf(`username: %s`, dataPrepper.Spec.Username))
				}),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, "password: test-password")
				}),
			))
		})
		It("should create a service", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataPrepper.Name,
					Namespace: testNs,
				},
			}
			Eventually(Object(service)).Should(ExistAnd(
				HaveOwner(dataPrepper),
				HavePorts(2021),
			))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(service), service)).To(Succeed())
			Expect(service.Spec.Selector).To(Equal(labels))
		})
		It("should create a deployment", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataPrepper.Name,
					Namespace: testNs,
				},
			}
			Eventually(Object(deploy)).Should(ExistAnd(
				HaveOwner(dataPrepper),
				HaveMatchingVolume(And(
					HaveName("config"),
					HaveVolumeSource("Secret"),
				)),
				HaveMatchingContainer(And(
					HaveImage("docker.io/opensearchproject/data-prepper:latest"),
					HaveVolumeMounts("config"),
				)),
			))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(deploy), deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Labels).To(Equal(labels))
		})
	})
	When("tracing is enabled", func() {
		Specify("update the dataprepper", func() {
			updateObject(dataPrepper, func(d *loggingv1beta1.DataPrepper) {
				d.Spec.EnableTracing = true
			})
		})
		It("should add tracing to the pipeline config", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-config", dataPrepper.Name),
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(dataPrepper),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, "entry-pipeline:")
				}),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, "raw-pipeline:")
				}),
				HaveData("pipelines.yaml", func(d string) bool {
					return strings.Contains(d, "service-map-pipeline:")
				}),
			))
		})
		It("should add the otel collector port to the service", func() {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataPrepper.Name,
					Namespace: testNs,
				},
			}
			Eventually(Object(service)).Should(ExistAnd(
				HaveOwner(dataPrepper),
				HavePorts(2021, 21890),
			))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(service), service)).To(Succeed())
			Expect(service.Spec.Selector).To(Equal(labels))
		})
	})
})

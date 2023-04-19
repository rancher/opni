package controllers

import (
	"context"
	"fmt"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Core Collector Controller", Ordered, Label("controller", "slow"), func() {
	var (
		ns           string
		config       *opniloggingv1beta1.CollectorConfig
		collectorObj *opnicorev1beta1.Collector
	)

	When("creating a collector resource", func() {
		It("should succeed", func() {
			ns = makeTestNamespace()
			config = &opniloggingv1beta1.CollectorConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-config",
				},
				Spec: opniloggingv1beta1.CollectorConfigSpec{
					Provider: opniloggingv1beta1.LogProviderGeneric,
				},
			}
			collectorObj = &opnicorev1beta1.Collector{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: opnicorev1beta1.CollectorSpec{
					SystemNamespace: ns,
					AgentEndpoint:   "http://test-endpoint",
					LoggingConfig: &corev1.LocalObjectReference{
						Name: config.Name,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), config)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), collectorObj)).To(Succeed())
		})
		// TODO: handle non happy path cases
		It("should create log scraping infra", func() {
			By("checking agent configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-agent-config", collectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("receivers.yaml", nil, "config.yaml", nil),
				HaveOwner(collectorObj),
			))
			By("checking aggregator configmap")
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-aggregator-config", collectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveData("aggregator.yaml", nil),
				HaveOwner(collectorObj),
			))
			By("checking daemonset")
			Eventually(Object(&appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-agent", collectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config", "varlogpods"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("HostPath"),
					HaveName("varlogpods"),
				)),
				HaveOwner(collectorObj),
			))
			By("checking deployment")
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-collector-aggregator", collectorObj.Name),
					Namespace: ns,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("otel-collector"),
					HaveVolumeMounts("collector-config"),
				)),
				HaveMatchingVolume(And(
					HaveVolumeSource("ConfigMap"),
					HaveName("collector-config"),
				)),
				HaveOwner(collectorObj),
			))
		})
	})
})

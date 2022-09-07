package controllers

import (
	"context"
	"fmt"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Core Nats Controller", Ordered, Label("controller"), func() {
	cluster := &opnicorev1beta1.NatsCluster{}

	Specify("setup", func() {
		cluster = &opnicorev1beta1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nats",
				Namespace: makeTestNamespace(),
			},
			Spec: opnicorev1beta1.NatsSpec{
				AuthMethod: opnicorev1beta1.NatsAuthPassword,
				NodeSelector: map[string]string{
					"foo": "bar",
				},
				Tolerations: []corev1.Toleration{
					{
						Key: "foo",
					},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
		Eventually(Object(cluster)).Should(Exist())
		Expect(k8sClient.Get(
			context.Background(),
			client.ObjectKeyFromObject(cluster),
			cluster,
		)).Should(Succeed())
	})

	It("should create a nats cluster", func() {
		By("checking nats statefulset")
		Eventually(Object(&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveReplicaCount(3),
			HaveNodeSelector("foo", "bar"),
			HaveTolerations("foo"),
			HaveOwner(cluster),
		))

		By("checking nats config secret")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-config", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("nats-config.conf", nil),
			HaveOwner(cluster),
		))

		By("checking nats headless service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-headless", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"opni.io/cluster-name", cluster.Name,
			),
			BeHeadless(),
			HaveOwner(cluster),
			HavePorts("tcp-client", "tcp-cluster"),
		))

		By("checking nats cluster service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-cluster", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"opni.io/cluster-name", cluster.Name,
			),
			HavePorts("tcp-cluster"),
			HaveType(corev1.ServiceTypeClusterIP),
			Not(BeHeadless()),
		))

		By("checking nats client service")
		Eventually(Object(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveOwner(cluster),
			HaveLabels(
				"app.kubernetes.io/name", "nats",
				"opni.io/cluster-name", cluster.Name,
			),
			HavePorts("tcp-client"),
			HaveType(corev1.ServiceTypeClusterIP),
			Not(BeHeadless()),
		))

		By("checking nats password secret")
		Eventually(Object(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-client", cluster.Name),
				Namespace: cluster.Namespace,
			},
		})).Should(ExistAnd(
			HaveData("password", nil),
			HaveOwner(cluster),
		))
	})
})

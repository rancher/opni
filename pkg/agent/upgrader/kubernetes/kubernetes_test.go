package kubernetes_test

import (
	"context"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/agent/upgrader/kubernetes"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	labels := map[string]string{
		"test": "test",
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-agent",
		},
	}
	agentDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-agent",
			Namespace: ns.Name,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "agent",
							Image: "opni.io/tobechanged:latest-minimal",
							Args:  []string{"agentv2"},
						},
						{
							Name:  "controllers",
							Image: "opni.io/tobechanged:latest",
							Args:  []string{"client"},
						},
					},
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil {
		return nil, err
	}
	if err := k8sClient.Create(ctx, agentDeployment); err != nil {
		return nil, err
	}
	return agentDeployment, nil
}

var _ = Describe("Kubernetes upgrader", Ordered, Label("integration"), func() {
	var deploy *appsv1.Deployment
	var driver *kubernetes.KubernetesAgentUpgrader

	BeforeAll(func() {
		var err error
		deploy, err = createDeployment(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		var err error
		driver, err = kubernetes.NewKubernetesAgentUpgrader(
			kubernetes.WithNamespace(deploy.Namespace),
			kubernetes.WithRestConfig(restConfig),
		)
		Expect(err).NotTo(HaveOccurred())
	})
	When("no manifests are sent", func() {
		It("should succeed", func() {
			err := driver.SyncAgent(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not update the deployment", func() {
			Consistently(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/tobechanged:latest-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("controllers"),
					HaveImage("opni.io/tobechanged:latest"),
				)),
			))
		})
	})
	When("the manifest items match the deployment", func() {
		It("should succeed", func() {
			err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
				{
					Package: "agent",
					Path:    "oci://opni.io/tobechanged",
					Digest:  "latest-minimal",
				},
				{
					Package: "client",
					Path:    "oci://opni.io/tobechanged",
					Digest:  "latest",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not update the deployment", func() {
			Consistently(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/tobechanged:latest-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("controllers"),
					HaveImage("opni.io/tobechanged:latest"),
				)),
			))
		})
	})
	When("just the agent manifest is sent", func() {
		It("should succeed", func() {
			err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
				{
					Package: "agent",
					Path:    "oci://opni.io/changed",
					Digest:  "tag1",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
		It("should just update the agent container", func() {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/changed:tag1"),
				)),
				HaveMatchingContainer(And(
					HaveName("controllers"),
					HaveImage("opni.io/tobechanged:latest"),
				)),
			))
		})
	})
	When("just the client manifest is sent", func() {
		It("should succeed", func() {
			err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
				{
					Package: "client",
					Path:    "oci://opni.io/changedtwo",
					Digest:  "tag2",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
		It("should just update both images", func() {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/changedtwo:tag2"),
				)),
				HaveMatchingContainer(And(
					HaveName("controllers"),
					HaveImage("opni.io/changedtwo:tag2"),
				)),
			))
		})
	})
	When("both manifests are sent", func() {
		It("should succeed", func() {
			err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
				{
					Package: "agent",
					Path:    "oci://opni/changedthree",
					Digest:  "tag3-minimal",
				},
				{
					Package: "client",
					Path:    "oci://opni/changedthree",
					Digest:  "tag3",
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
		It("should just update both images", func() {
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni/changedthree:tag3-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("controllers"),
					HaveImage("opni/changedthree:tag3"),
				)),
			))
		})
	})
	When("a repo override is specified", func() {
		BeforeEach(func() {
			var err error
			driver, err = kubernetes.NewKubernetesAgentUpgrader(
				kubernetes.WithNamespace(deploy.Namespace),
				kubernetes.WithRestConfig(restConfig),
				kubernetes.WithRepoOverride(lo.ToPtr("test.io")),
			)
			Expect(err).NotTo(HaveOccurred())
		})
		When("manifests don't have a repo", func() {
			It("should succeed", func() {
				err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
					{
						Package: "agent",
						Path:    "oci://opni/changedthree",
						Digest:  "tag3-minimal",
					},
					{
						Package: "client",
						Path:    "oci://opni/changedthree",
						Digest:  "tag3",
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("both images should include the repo override", func() {
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      deploy.Name,
						Namespace: deploy.Namespace,
					},
				})).Should(ExistAnd(
					HaveMatchingContainer(And(
						HaveName("agent"),
						HaveImage("test.io/opni/changedthree:tag3-minimal"),
					)),
					HaveMatchingContainer(And(
						HaveName("controllers"),
						HaveImage("test.io/opni/changedthree:tag3"),
					)),
				))
			})
		})
		When("manifests do have a repo", func() {
			It("should succeed", func() {
				err := driver.SyncAgent(context.Background(), []*controlv1.UpdateManifestEntry{
					{
						Package: "agent",
						Path:    "oci://opni.io/test/changedthree",
						Digest:  "tag3-minimal",
					},
					{
						Package: "client",
						Path:    "oci://opni.io/test/changedthree",
						Digest:  "tag3",
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
			It("both images should include the repo override", func() {
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      deploy.Name,
						Namespace: deploy.Namespace,
					},
				})).Should(ExistAnd(
					HaveMatchingContainer(And(
						HaveName("agent"),
						HaveImage("test.io/test/changedthree:tag3-minimal"),
					)),
					HaveMatchingContainer(And(
						HaveName("controllers"),
						HaveImage("test.io/test/changedthree:tag3"),
					)),
				))
			})
		})
	})
})

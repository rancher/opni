package client_test

import (
	"context"
	"fmt"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/kubernetes"
	"github.com/rancher/opni/pkg/update/kubernetes/client"
	"github.com/rancher/opni/pkg/urn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	imageDigest = "sha256:15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"
)

var _ = Describe("Kubernetes update client", Label("unit", "slow"), func() {
	var (
		deploy       *appsv1.Deployment
		updateClient update.SyncHandler
		syncResp     *controlv1.SyncResults
	)

	BeforeEach(func() {
		labels := map[string]string{
			"opni.io/app": "agent",
		}
		deploy = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deploy",
				Namespace: namespace,
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
								Args:  []string{"agentv2"},
								Image: "opni.io/test:latest-minimal",
							},
							{
								Name: "client",
								Args: []string{
									"client",
									"--health-probe-bind-address=:7081",
									"--metrics-bind-address=127.0.0.1:7080",
								},
								Image: "opni.io/test:latest",
							},
						},
					},
				},
			},
		}
		var err error
		updateClient, err = client.NewKubernetesAgentUpgrader(testlog.Log, client.WithRestConfig(restConfig), client.WithNamespace(namespace))
		Expect(err).NotTo(HaveOccurred())
	})
	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.Background(), deploy)).To(Succeed())
		Eventually(k8sClient.Get(context.Background(), ctrlclient.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
	})
	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), deploy)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(context.Background(), ctrlclient.ObjectKeyFromObject(deploy), deploy)
			if err == nil {
				return false
			}
			return k8serrors.IsNotFound(err)
		}).Should(BeTrue())
	})

	Specify("client should calculate the existing manifest correctly", func() {
		manifest, err := updateClient.GetCurrentManifest(context.Background())
		Expect(err).NotTo(HaveOccurred())
		updateType, err := update.GetType(manifest.GetItems())
		Expect(err).NotTo(HaveOccurred())
		Expect(updateType).To(Equal(urn.Agent))
		Expect(len(manifest.GetItems())).To(Equal(2))
		for _, item := range manifest.GetItems() {
			if item.Package == "urn:opni:agent:kubernetes:agent" {
				Expect(item.GetPath()).To(Equal("opni.io/test"))
				Expect(item.GetDigest()).To(Equal("latest-minimal"))
			}
			if item.Package == "urn:opni:agent:kubernetes:controller" {
				Expect(item.GetPath()).To(Equal("opni.io/test"))
				Expect(item.GetDigest()).To(Equal("latest"))
			}
		}
	})

	When("sync result has an plugin package", func() {
		BeforeEach(func() {
			packageURN := urn.NewOpniURN(urn.Plugin, kubernetes.UpdateStrategy, "agent")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN.String(),
						Path:    "opni.io/test",
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				RequiredPatches: patchList,
			}
		})
		It("should return an error", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
	When("sync result has an unknown package", func() {
		BeforeEach(func() {
			packageURN := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "unknown")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN.String(),
						Path:    "opni.io/test",
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				DesiredState:    manifest,
				RequiredPatches: patchList,
			}
		})
		It("should do nothing", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(err).NotTo(HaveOccurred())
			Consistently(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/test:latest-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("client"),
					HaveImage("opni.io/test:latest"),
				)),
			))
		})
	})
	When("sync result has noop packages", func() {
		BeforeEach(func() {
			packageURN1 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "agent")
			packageURN2 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "controller")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN1.String(),
						Path:    "opni.io/test",
						Digest:  "latest-minimal",
					},
					{
						Package: packageURN2.String(),
						Path:    "opni.io/test",
						Digest:  "latest",
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN1.String(),
						Path:    "opni.io/test",
					},
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN2.String(),
						Path:    "opni.io/test",
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				DesiredState:    manifest,
				RequiredPatches: patchList,
			}
		})
		It("should do nothing", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(err).NotTo(HaveOccurred())
			Consistently(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/test:latest-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("client"),
					HaveImage("opni.io/test:latest"),
				)),
			))
		})
	})
	When("agent package has an update", func() {
		BeforeEach(func() {
			packageURN1 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "agent")
			packageURN2 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "controller")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN1.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
					{
						Package: packageURN2.String(),
						Path:    "opni.io/test",
						Digest:  "latest",
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:        controlv1.PatchOp_Update,
						Package:   packageURN1.String(),
						Path:      "opni.io/test",
						OldDigest: "latest-minimal",
						NewDigest: imageDigest,
					},
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN2.String(),
						Path:    "opni.io/test",
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				DesiredState:    manifest,
				RequiredPatches: patchList,
			}
		})
		It("should just update the agent", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(err).NotTo(HaveOccurred())
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage(fmt.Sprintf("opni.io/test@%s", imageDigest)),
				)),
				HaveMatchingContainer(And(
					HaveName("client"),
					HaveImage("opni.io/test:latest"),
				)),
			))
		})
	})
	When("controller package has an update", func() {
		BeforeEach(func() {
			packageURN1 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "agent")
			packageURN2 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "controller")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN1.String(),
						Path:    "opni.io/test",
						Digest:  "latest-minimal",
					},
					{
						Package: packageURN2.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:        controlv1.PatchOp_Update,
						Package:   packageURN2.String(),
						Path:      "opni.io/test",
						OldDigest: "latest",
						NewDigest: imageDigest,
					},
					{
						Op:      controlv1.PatchOp_None,
						Package: packageURN1.String(),
						Path:    "opni.io/test",
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				DesiredState:    manifest,
				RequiredPatches: patchList,
			}
		})
		It("should just update the controller", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(err).NotTo(HaveOccurred())
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage("opni.io/test:latest-minimal"),
				)),
				HaveMatchingContainer(And(
					HaveName("client"),
					HaveImage(fmt.Sprintf("opni.io/test@%s", imageDigest)),
				)),
			))
		})
	})
	When("both packages have an update", func() {
		BeforeEach(func() {
			packageURN1 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "agent")
			packageURN2 := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "controller")
			manifest := &controlv1.UpdateManifest{
				Items: []*controlv1.UpdateManifestEntry{
					{
						Package: packageURN1.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
					{
						Package: packageURN2.String(),
						Path:    "opni.io/test",
						Digest:  imageDigest,
					},
				},
			}
			patchList := &controlv1.PatchList{
				Items: []*controlv1.PatchSpec{
					{
						Op:        controlv1.PatchOp_Update,
						Package:   packageURN1.String(),
						Path:      "opni.io/test",
						OldDigest: "latest-minimal",
						NewDigest: imageDigest,
					},
					{
						Op:        controlv1.PatchOp_Update,
						Package:   packageURN2.String(),
						Path:      "opni.io/test",
						OldDigest: "latest",
						NewDigest: imageDigest,
					},
				},
			}
			syncResp = &controlv1.SyncResults{
				DesiredState:    manifest,
				RequiredPatches: patchList,
			}
		})
		It("should just update the controller", func() {
			err := updateClient.HandleSyncResults(context.Background(), syncResp)
			Expect(err).NotTo(HaveOccurred())
			Eventually(Object(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploy.Name,
					Namespace: deploy.Namespace,
				},
			})).Should(ExistAnd(
				HaveMatchingContainer(And(
					HaveName("agent"),
					HaveImage(fmt.Sprintf("opni.io/test@%s", imageDigest)),
				)),
				HaveMatchingContainer(And(
					HaveName("client"),
					HaveImage(fmt.Sprintf("opni.io/test@%s", imageDigest)),
				)),
			))
		})
	})
})

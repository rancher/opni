package server_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	_ "github.com/rancher/opni/pkg/oci/noop"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/kubernetes"
	"github.com/rancher/opni/pkg/update/kubernetes/server"
	"github.com/rancher/opni/pkg/urn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	imageDigest = "sha256:15e2b0d3c33891ebb0f1ef609ec419420c20e320ce94c65fbc8c3312448eb225"
)

var _ = Describe("Kubernetes sync server", Label("unit"), func() {
	var k8sServer update.UpdateTypeHandler
	incomingContext := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		controlv1.UpdateStrategyKeyForType(urn.Agent), "noop",
	))

	BeforeEach(func() {
		var err error
		k8sServer, err = server.NewKubernetesSyncServer(v1beta1.KubernetesAgentUpgradeSpec{
			ImageResolver: "noop",
		}, testlog.Log)
		Expect(err).NotTo(HaveOccurred())
	})

	When("unknown package type is provided", func() {
		packageURN := urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "unknown")
		manifest := &controlv1.UpdateManifest{
			Items: []*controlv1.UpdateManifestEntry{
				{
					Package: packageURN.String(),
					Path:    "rancher/opni",
					Digest:  "latest",
				},
			},
		}
		It("should return an error", func() {
			_, err := k8sServer.CalculateUpdate(incomingContext, manifest)
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})
	})
	When("an invalid URN is provided", func() {
		manifest := &controlv1.UpdateManifest{
			Items: []*controlv1.UpdateManifestEntry{
				{
					Package: "urn:malformed",
					Path:    "rancher/opni",
					Digest:  "latest",
				},
			},
		}
		It("should return an error", func() {
			_, err := k8sServer.CalculateUpdate(incomingContext, manifest)
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})
	})
	When("URNs are valid", func() {
		var packageURN1, packageURN2 urn.OpniURN
		var manifest *controlv1.UpdateManifest
		BeforeEach(func() {
			packageURN1 = urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "agent")
			packageURN2 = urn.NewOpniURN(urn.Agent, kubernetes.UpdateStrategy, "controller")
		})
		When("manifest matches the current version", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "example.io/opni-noop",
							Digest:  imageDigest,
						},
						{
							Package: packageURN2.String(),
							Path:    "example.io/opni-noop",
							Digest:  imageDigest,
						},
					},
				}
			})
			It("should return noop updates", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				Expect(patchList.GetItems()).To(HaveLen(2))
				Expect(patchList.Items[0].GetOp()).To(Equal(controlv1.PatchOp_None))
				Expect(patchList.Items[1].GetOp()).To(Equal(controlv1.PatchOp_None))
				Expect(patchList.Items[0].GetPackage()).To(Equal(packageURN1.String()))
				Expect(patchList.Items[1].GetPackage()).To(Equal(packageURN2.String()))
			})
		})
		When("digest matches the current version but registry is different", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "quay.io/opni-noop",
							Digest:  imageDigest,
						},
						{
							Package: packageURN2.String(),
							Path:    "opni-noop",
							Digest:  imageDigest,
						},
					},
				}
			})
			It("should return noop updates", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				Expect(patchList.GetItems()).To(HaveLen(2))
				Expect(patchList.Items[0].GetOp()).To(Equal(controlv1.PatchOp_None))
				Expect(patchList.Items[1].GetOp()).To(Equal(controlv1.PatchOp_None))
				Expect(patchList.Items[0].GetPackage()).To(Equal(packageURN1.String()))
				Expect(patchList.Items[1].GetPackage()).To(Equal(packageURN2.String()))
			})
		})
		When("one digest does not match", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "example.io/opni-noop",
							Digest:  imageDigest,
						},
						{
							Package: packageURN2.String(),
							Path:    "example.io/opni-noop",
							Digest:  "latest",
						},
					},
				}
			})
			It("should return one change update", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				for _, patch := range patchList.GetItems() {
					if patch.GetPackage() == packageURN2.String() {
						Expect(patch.GetOp()).To(Equal(controlv1.PatchOp_Update))
						Expect(patch.GetPath()).To(Equal("example.io/opni-noop"))
						Expect(patch.GetNewDigest()).To(Equal(imageDigest))
						Expect(patch.GetOldDigest()).To(Equal("latest"))
					}
				}
			})
			It("should return one noop update", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				Expect(func() bool {
					for _, patch := range patchList.GetItems() {
						if patch.GetOp() == controlv1.PatchOp_None {
							return true
						}
					}
					return false
				}()).To(BeTrue())
			})
		})
		When("one digest does not match and the registry is different", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "example.io/opni-noop",
							Digest:  imageDigest,
						},
						{
							Package: packageURN2.String(),
							Path:    "opni.io/opni-noop",
							Digest:  "latest",
						},
					},
				}
			})
			It("should return one change update with the registry changed", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				for _, patch := range patchList.GetItems() {
					if patch.GetPackage() == packageURN2.String() {
						Expect(patch.GetOp()).To(Equal(controlv1.PatchOp_Update))
						Expect(patch.GetPath()).To(Equal("example.io/opni-noop"))
						Expect(patch.GetNewDigest()).To(Equal(imageDigest))
						Expect(patch.GetOldDigest()).To(Equal("latest"))
					}
				}
			})
		})
		When("image repository is different", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "example.io/opni-noop",
							Digest:  imageDigest,
						},
						{
							Package: packageURN2.String(),
							Path:    "example.io/rancher/test",
							Digest:  imageDigest,
						},
					},
				}
			})
			It("should return one change update with the correct repo", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				for _, patch := range patchList.GetItems() {
					if patch.GetPackage() == packageURN2.String() {
						Expect(patch.GetOp()).To(Equal(controlv1.PatchOp_Update))
						Expect(patch.GetPath()).To(Equal("example.io/opni-noop"))
						Expect(patch.GetNewDigest()).To(Equal(imageDigest))
						Expect(patch.GetOldDigest()).To(Equal(imageDigest))
					}
				}
			})
		})
		When("both images should be updated", func() {
			JustBeforeEach(func() {
				manifest = &controlv1.UpdateManifest{
					Items: []*controlv1.UpdateManifestEntry{
						{
							Package: packageURN1.String(),
							Path:    "example.io/opni-noop",
							Digest:  "latest",
						},
						{
							Package: packageURN2.String(),
							Path:    "example.io/rancher/test",
							Digest:  imageDigest,
						},
					},
				}
			})
			It("should return patch updates", func() {
				patchList, err := k8sServer.CalculateUpdate(incomingContext, manifest)
				Expect(err).NotTo(HaveOccurred())
				Expect(func() bool {
					for _, patch := range patchList.GetItems() {
						if patch.GetOp() != controlv1.PatchOp_Update {
							return false
						}
					}
					return true
				}()).To(BeTrue())
			})
		})
	})
})

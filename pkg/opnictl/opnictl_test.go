package opnictl_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/pkg/opnictl"
	"github.com/rancher/opni/pkg/opnictl/common"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

var _ = Describe("Opnictl Commands", func() {
	var _ = Describe("Install", func() {
		It("should install the manager with the install command", func() {
			os.Args = []string{"opnictl", "install"}
			err := opnictl.BuildRootCmd().Execute()
			Expect(err).NotTo(HaveOccurred())
		})
		It("should create the necessary resources", func() {
			errors := cliutil.ForEachStagingResource(common.RestConfig,
				func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
					Eventually(func() error {
						_, err := dr.Get(context.Background(), obj.GetName(), v1.GetOptions{})
						return err
					}).Should(BeNil())
					return nil
				},
			)
			Expect(errors).To(BeEmpty())
		})
	})
	var _ = Describe("Create", func() {
		var _ = Describe("Demo API", func() {
			It("should create a new demo custom resource with default values", func() {
				os.Args = []string{"opnictl", "create", "demo"}
				ctx, ca := context.WithCancel(context.Background())
				completed := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					defer close(completed)
					err := opnictl.BuildRootCmd().ExecuteContext(ctx)
					Expect(err).To(Or(BeNil(), MatchError(context.Canceled)))
				}()
				demoCR := v1alpha1.OpniDemo{}
				Eventually(func() error {
					return common.K8sClient.Get(ctx, types.NamespacedName{
						Namespace: "opni-demo",
						Name:      "opni-demo",
					}, &demoCR)
				}, 5*time.Second, 100*time.Millisecond).Should(BeNil())
				select {
				case <-completed:
				case <-time.After(2 * time.Second):
				}
				ca()
				// Check that the values all equal the defaults
				Expect(demoCR.ObjectMeta.Name).To(Equal(common.DefaultOpniDemoName))
				Expect(demoCR.ObjectMeta.Namespace).To(Equal(common.DefaultOpniDemoNamespace))
				Expect(demoCR.Spec.MinioAccessKey).To(Equal(common.DefaultOpniDemoMinioAccessKey))
				Expect(demoCR.Spec.MinioSecretKey).To(Equal(common.DefaultOpniDemoMinioSecretKey))
				Expect(demoCR.Spec.MinioVersion).To(Equal(common.DefaultOpniDemoMinioVersion))
				Expect(demoCR.Spec.NatsVersion).To(Equal(common.DefaultOpniDemoNatsVersion))
				Expect(demoCR.Spec.NatsPassword).To(Equal(common.DefaultOpniDemoNatsPassword))
				Expect(demoCR.Spec.NatsReplicas).To(Equal(common.DefaultOpniDemoNatsReplicas))
				Expect(demoCR.Spec.NatsMaxPayload).To(Equal(common.DefaultOpniDemoNatsMaxPayload))
				Expect(demoCR.Spec.NvidiaVersion).To(Equal(common.DefaultOpniDemoNvidiaVersion))
				Expect(demoCR.Spec.ElasticsearchUser).To(Equal(common.DefaultOpniDemoElasticUser))
				Expect(demoCR.Spec.ElasticsearchPassword).To(Equal(common.DefaultOpniDemoElasticPassword))
				Expect(demoCR.Spec.NulogServiceCPURequest).To(Equal(common.DefaultOpniDemoNulogServiceCPURequest))
				Expect(demoCR.Spec.Quickstart).To(Equal(common.DefaultOpniDemoQuickstart))
			})
			It("should create a new demo custom resource with user-specified values", func() {
				os.Args = []string{"opnictl", "create", "demo",
					"--name=custom",
					"--namespace=kube-system",
					"--minio-access-key=customAccessKey",
					"--minio-secret-key=customSecretKey",
					"--minio-version=100",
					"--nats-version=200",
					"--nats-password=natsPassword",
					"--nats-replicas=99",
					"--nats-max-payload=1234567",
					"--nvidia-version=300",
					"--elasticsearch-user=todo-this-flag-probably-does-nothing",
					"--elasticsearch-password=todo-this-flag-probably-does-nothing",
					"--nulog-service-cpu-request=999",
					"--quickstart",
				}
				ctx, ca := context.WithCancel(context.Background())
				completed := make(chan struct{})
				go func() {
					defer GinkgoRecover()
					defer close(completed)
					err := opnictl.BuildRootCmd().ExecuteContext(ctx)
					Expect(err).To(Or(BeNil(), MatchError(context.Canceled)))
				}()
				demoCR := v1alpha1.OpniDemo{}
				Eventually(func() error {
					return common.K8sClient.Get(ctx, types.NamespacedName{
						Namespace: "kube-system",
						Name:      "custom",
					}, &demoCR)
				}, 5*time.Second, 100*time.Millisecond).Should(BeNil())
				select {
				case <-completed:
				case <-time.After(2 * time.Second):
				}
				ca()
				// Check that the values all equal the custom user-supplied ones
				Expect(demoCR.ObjectMeta.Name).To(Equal("custom"))
				Expect(demoCR.ObjectMeta.Namespace).To(Equal("kube-system"))
				Expect(demoCR.Spec.MinioAccessKey).To(Equal("customAccessKey"))
				Expect(demoCR.Spec.MinioSecretKey).To(Equal("customSecretKey"))
				Expect(demoCR.Spec.MinioVersion).To(Equal("100"))
				Expect(demoCR.Spec.NatsVersion).To(Equal("200"))
				Expect(demoCR.Spec.NatsPassword).To(Equal("natsPassword"))
				Expect(demoCR.Spec.NatsReplicas).To(Equal(99))
				Expect(demoCR.Spec.NatsMaxPayload).To(Equal(1234567))
				Expect(demoCR.Spec.NvidiaVersion).To(Equal("300"))
				Expect(demoCR.Spec.ElasticsearchUser).To(Equal("todo-this-flag-probably-does-nothing"))
				Expect(demoCR.Spec.ElasticsearchPassword).To(Equal("todo-this-flag-probably-does-nothing"))
				Expect(demoCR.Spec.NulogServiceCPURequest).To(Equal("999"))
				Expect(demoCR.Spec.Quickstart).To(Equal(true))
			})
		})
	})
	var _ = Describe("Get", func() {
		var _ = Describe("Demo API", func() {
			It("should list the existing custom resource objects", func() {
				r, w, _ := os.Pipe()
				oldStdout := os.Stdout
				os.Stdout = w
				defer func() {
					os.Stdout = oldStdout
				}()
				os.Args = []string{"opnictl", "get", "opnidemoes"}
				ctx, ca := context.WithTimeout(context.Background(), 5*time.Second)
				defer ca()
				err := opnictl.BuildRootCmd().ExecuteContext(ctx)
				Expect(err).NotTo(HaveOccurred())

				buf := make([]byte, 1024)
				_, err = r.Read(buf)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(buf)).To(And(
					ContainSubstring("opni-demo"),
					ContainSubstring("Unknown"),
				))
			})
		})
	})
	var _ = Describe("Delete", func() {
		var _ = Describe("Demo API", func() {
			It("should delete an existing custom resource object", func() {
				os.Args = []string{"opnictl", "delete", "demo",
					"custom",
					"--namespace=kube-system",
				}
				ctx, ca := context.WithTimeout(context.Background(), 5*time.Second)
				defer ca()
				err := opnictl.BuildRootCmd().ExecuteContext(ctx)
				Expect(err).NotTo(HaveOccurred())

				demoCR := v1alpha1.OpniDemo{}
				Eventually(func() error {
					return common.K8sClient.Get(ctx, types.NamespacedName{
						Namespace: "kube-system",
						Name:      "custom",
					}, &demoCR)
				}, 5*time.Second, 100*time.Millisecond).
					Should(WithTransform(errors.IsNotFound, BeTrue()))
			})
			Specify("non-deleted object should still exist", func() {
				ctx, ca := context.WithTimeout(context.Background(), 5*time.Second)
				defer ca()
				demoCR := v1alpha1.OpniDemo{}
				Expect(common.K8sClient.Get(ctx, types.NamespacedName{
					Namespace: "opni-demo",
					Name:      "opni-demo",
				}, &demoCR))
			})
		})
	})
	var _ = Describe("uninstall", func() {
		It("should complete successfully", func() {
			os.Args = []string{"opnictl", "uninstall"}
			ctx, ca := context.WithTimeout(context.Background(), 5*time.Second)
			defer ca()
			err := opnictl.BuildRootCmd().ExecuteContext(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should delete all opni resources", func() {
			Eventually(func() bool {
				notFoundErrors := 0
				total := 0
				cliutil.ForEachStagingResource(common.RestConfig,
					func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
						total++
						_, err := dr.Get(context.Background(), obj.GetName(), v1.GetOptions{})
						if errors.IsNotFound(err) || obj.GetKind() == "Namespace" {
							notFoundErrors++
						}
						return err
					},
				)

				return notFoundErrors == total
			}, 30*time.Minute, 500*time.Millisecond).Should(BeTrue())
		})
	})
})

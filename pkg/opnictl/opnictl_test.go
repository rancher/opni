package opnictl_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/opnictl"
	"github.com/rancher/opni/pkg/opnictl/common"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var _ = Describe("Opnictl Commands", Label("e2e", "gpu", "demo"), func() {
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
	var _ = Describe("uninstall", func() {
		It("should complete successfully", func() {
			// Can't run finalizers in the test environment, so we need to use the
			// Background propagation policy instead of the default Foreground
			os.Args = []string{"opnictl", "uninstall", "--deletion-propagation-policy=Background"}
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
			}, 1*time.Minute, 500*time.Millisecond).Should(BeTrue())
		})
	})
})

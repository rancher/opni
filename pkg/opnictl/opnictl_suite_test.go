//go:build !e2e
// +build !e2e

package opnictl_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/opnictl"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// These tests are only verifying that opnictl correctly interacts with the
// opni custom resources, *not* that the manager correctly reconciles them.
// As such, these tests do not run the manager inside the controller-runtime
// environment.

var testEnv *envtest.Environment

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Commands Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(util.NewTestLogger())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../config/crd/bases",
			"../../test/resources",
		},
		BinaryAssetsDirectory: "../../testbin/bin",
	}

	// Set up the common globals that the commands use
	var err error
	common.RestConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	common.K8sClient, err = client.New(common.RestConfig, client.Options{
		Scheme: opnictl.CreateScheme(),
	})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

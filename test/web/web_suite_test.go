package web_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/onsi/biloba"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	_ "github.com/rancher/opni/pkg/test/setup"
	"golang.org/x/net/context"
)

func TestWeb(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Web Suite")
}

var b *biloba.Biloba
var url string
var env *test.Environment
var mgmtClient managementv1.ManagementClient

var _ = SynchronizedBeforeSuite(func() {
	biloba.SpinUpChrome(GinkgoT())
}, func(ctx context.Context) {
	env = &test.Environment{}
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)
	b = biloba.ConnectToChrome(GinkgoT())
	url = "http://" + env.GatewayConfig().Spec.Management.WebListenAddress
	mgmtClient = env.NewManagementClient()
})

var _ = BeforeEach(func() {
	b.Prepare()
}, OncePerOrdered)

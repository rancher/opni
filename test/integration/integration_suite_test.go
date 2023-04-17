package integration_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/rancher/opni/plugins/example/test"
)

func TestIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

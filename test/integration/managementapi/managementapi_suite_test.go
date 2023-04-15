package integration_test

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/rancher/opni/plugins/example/test"
)

func TestManagementapi(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetDefaultEventuallyTimeout(30 * time.Second)
	RegisterFailHandler(Fail)
	RunSpecs(t, "ManagementAPI Suite")
}

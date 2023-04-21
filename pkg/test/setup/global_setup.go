package setup

import (
	"github.com/gin-gonic/gin"
	"github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
	logger.DefaultWriter = ginkgo.GinkgoWriter
}

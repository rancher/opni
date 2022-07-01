package plugins_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plugins Suite")
}

// SLO plugin
// - create client
// - test public plugin interfaces functional
// - test side effects (e.g cortex rules)

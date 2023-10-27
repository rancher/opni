package alerting_manager_test

import (
	"testing"

	_ "github.com/rancher/opni/pkg/test/setup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAlertingManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AlertingManager Suite")
}

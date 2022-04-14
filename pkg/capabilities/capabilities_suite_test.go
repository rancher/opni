package capabilities_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/core"
)

var ctrl *gomock.Controller

func TestCapabilities(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Capabilities Suite")
}

var _ = BeforeSuite(func() {
	ctrl = gomock.NewController(GinkgoT())
})

type testCapability string

func (tc testCapability) Equal(other testCapability) bool {
	return tc == other
}

type testResourceWithMetadata struct {
	capabilities []testCapability
	labels       map[string]string
}

var _ core.MetadataAccessor[testCapability] = (*testResourceWithMetadata)(nil)

func (t *testResourceWithMetadata) GetCapabilities() []testCapability {
	return t.capabilities
}

func (t *testResourceWithMetadata) SetCapabilities(capabilities []testCapability) {
	t.capabilities = capabilities
}

func (t *testResourceWithMetadata) GetLabels() map[string]string {
	return t.labels
}

func (t *testResourceWithMetadata) SetLabels(labels map[string]string) {
	t.labels = labels
}

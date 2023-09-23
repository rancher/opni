package inmemory_test

import (
	"bytes"
	"testing"

	"github.com/rancher/opni/pkg/storage"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/util/future"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/inmemory"
)

func TestInmemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Inmemory Suite")
}

type testBroker struct{}

func (t testBroker) KeyValueStore(string) storage.KeyValueStore {
	return inmemory.NewKeyValueStore(bytes.Clone)
}

var _ = Describe("In-memory KV Store", Ordered, Label("integration"), KeyValueStoreTestSuite(future.Instant(testBroker{}), NewBytes, Equal))

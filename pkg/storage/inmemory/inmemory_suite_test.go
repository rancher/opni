package inmemory_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/storage/inmemory"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/util/future"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var lmF = future.New[*inmemory.InMemoryLockManager]()

func TestInmemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Inmemory Suite")
}

type testBroker struct{}

func (t testBroker) LockManager(path string) *inmemory.InMemoryLockManager {
	return inmemory.NewLockManager(context.TODO(), path)
}

func init() {
	t := testBroker{}
	lmF.Set(t.LockManager(fmt.Sprintf("/tmp/lock/opni-test-%s", uuid.New().String())))
}

var _ = Describe("In-memory Lock Manager", Ordered, Label("integration"), LockManagerTestSuite(lmF))

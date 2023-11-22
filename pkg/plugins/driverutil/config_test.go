package driverutil_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	conformance_driverutil "github.com/rancher/opni/pkg/test/conformance/driverutil"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func newValueStore() storage.ValueStoreT[*ext.SampleConfiguration] {
	return inmemory.NewValueStore[*ext.SampleConfiguration](util.ProtoClone)
}

func newKeyValueStore() storage.KeyValueStoreT[*ext.SampleConfiguration] {
	return inmemory.NewKeyValueStore[*ext.SampleConfiguration](util.ProtoClone)
}

var _ = Describe("Defaulting Config Tracker", Label("unit"), conformance_driverutil.DefaultingConfigTrackerTestSuite(newValueStore, newValueStore))

type testContextKey struct {
	*corev1.ClusterStatus
}

func (t testContextKey) ContextKey() protoreflect.FieldDescriptor {
	return t.ProtoReflect().Descriptor().Fields().ByName("cluster")
}

var _ = Describe("Context Keys", func() {
	It("should correctly obtain context key values from ContextKeyable messages", func() {
		getReq := &ext.SampleGetRequest{
			Key: lo.ToPtr("foo"),
		}
		ctx := context.Background()
		ctx = driverutil.ContextWithKey(ctx, getReq)
		key := driverutil.KeyFromContext(ctx)

		Expect(key).To(Equal("foo"))
	})
	It("should correctly obtain context key values if the key field is a core.Reference", func() {
		testKeyable := &testContextKey{
			ClusterStatus: &corev1.ClusterStatus{
				Cluster: &corev1.Reference{
					Id: "foo",
				},
			},
		}
		ctx := context.Background()
		ctx = driverutil.ContextWithKey(ctx, testKeyable)
		key := driverutil.KeyFromContext(ctx)

		Expect(key).To(Equal("foo"))
	})
})

package driverutil_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	conformance_driverutil "github.com/rancher/opni/pkg/test/conformance/driverutil"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util"
)

func newValueStore() storage.ValueStoreT[*ext.SampleConfiguration] {
	return inmemory.NewValueStore[*ext.SampleConfiguration](util.ProtoClone)
}

var _ = Describe("Defaulting Config Tracker", Label("unit"), conformance_driverutil.DefaultingConfigTrackerTestSuite(newValueStore, newValueStore))

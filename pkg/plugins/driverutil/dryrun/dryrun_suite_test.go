package dryrun_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util"
)

func TestDryrun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DryRun Suite")
}

func newValueStore() storage.ValueStoreT[*ext.SampleConfiguration] {
	return inmemory.NewValueStore[*ext.SampleConfiguration](util.ProtoClone)
}

func newKeyValueStore() storage.KeyValueStoreT[*ext.SampleConfiguration] {
	return inmemory.NewKeyValueStore[*ext.SampleConfiguration](util.ProtoClone)
}

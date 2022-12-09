package patch_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/patch"
	"github.com/rancher/opni/pkg/test/testutil"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/errgroup"
)

func TestPatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Patch Suite")
}

var (
	test1Module = "github.com/rancher/opni/pkg/test/testdata/patch/test1"
	test2Module = "github.com/rancher/opni/pkg/test/testdata/patch/test2"
	testModules = map[string]string{
		"test1": test1Module,
		"test2": test2Module,
	}

	test1v1BinaryPath = new(string)
	test1v2BinaryPath = new(string)
	test2v1BinaryPath = new(string)
	test2v2BinaryPath = new(string)

	testBinaries = map[string]map[string]*string{
		"test1": {
			"v1": test1v1BinaryPath,
			"v2": test1v2BinaryPath,
		},
		"test2": {
			"v1": test2v1BinaryPath,
			"v2": test2v2BinaryPath,
		},
	}

	test1v1tov2Patch = new(bytes.Buffer)
	test2v1tov2Patch = new(bytes.Buffer)

	testPatches = map[string]*bytes.Buffer{
		"test1": test1v1tov2Patch,
		"test2": test2v1tov2Patch,
	}
)

func b2sum(filename string) string {
	contents, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	sum := blake2b.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

var v1Manifest *controlv1.PluginArchive
var v2Manifest *controlv1.PluginArchive

var ctrl *gomock.Controller

var _ = BeforeSuite(func() {
	ctrl = gomock.NewController(GinkgoT())

	var eg errgroup.Group

	eg.Go(func() error {
		var err error
		*test1v1BinaryPath, err = gexec.Build(test1Module, "-tags=v1")
		return err
	})
	eg.Go(func() error {
		var err error
		*test1v2BinaryPath, err = gexec.Build(test1Module, "-tags=v2")
		return err
	})
	eg.Go(func() error {
		var err error
		*test2v1BinaryPath, err = gexec.Build(test2Module, "-tags=v1")
		return err
	})
	eg.Go(func() error {
		var err error
		*test2v2BinaryPath, err = gexec.Build(test2Module, "-tags=v2")
		return err
	})
	Expect(eg.Wait()).To(Succeed())

	patcher := patch.BsdiffPatcher{}
	eg = errgroup.Group{}

	eg.Go(func() error {
		test1v1, err := os.Open(*test1v1BinaryPath)
		if err != nil {
			return err
		}
		defer test1v1.Close()
		test1v2, err := os.Open(*test1v2BinaryPath)
		if err != nil {
			return err
		}
		defer test1v2.Close()

		return patcher.GeneratePatch(test1v1, test1v2, test1v1tov2Patch)
	})

	eg.Go(func() error {
		test2v1, err := os.Open(*test2v1BinaryPath)
		if err != nil {
			return err
		}
		defer test2v1.Close()
		test2v2, err := os.Open(*test2v2BinaryPath)
		if err != nil {
			return err
		}
		defer test2v2.Close()

		return patcher.GeneratePatch(test2v1, test2v2, test2v1tov2Patch)
	})

	Expect(eg.Wait()).To(Succeed())

	v1Manifest = &controlv1.PluginArchive{
		Items: []*controlv1.PluginArchiveEntry{
			{
				Metadata: &controlv1.PluginManifestEntry{
					Module:   test1Module,
					Filename: "test1",
					Digest:   b2sum(*test1v1BinaryPath),
				},
				Data: testutil.Must(os.ReadFile(*test1v1BinaryPath)),
			},
			{
				Metadata: &controlv1.PluginManifestEntry{
					Module:   test2Module,
					Filename: "test2",
					Digest:   b2sum(*test2v1BinaryPath),
				},
				Data: testutil.Must(os.ReadFile(*test2v1BinaryPath)),
			},
		},
	}
	v2Manifest = &controlv1.PluginArchive{
		Items: []*controlv1.PluginArchiveEntry{
			{
				Metadata: &controlv1.PluginManifestEntry{
					Module:   test1Module,
					Filename: "test1",
					Digest:   b2sum(*test1v2BinaryPath),
				},
				Data: testutil.Must(os.ReadFile(*test1v2BinaryPath)),
			},
			{
				Metadata: &controlv1.PluginManifestEntry{
					Module:   test2Module,
					Filename: "test2",
					Digest:   b2sum(*test2v2BinaryPath),
				},
				Data: testutil.Must(os.ReadFile(*test2v2BinaryPath)),
			},
		},
	}

	DeferCleanup(func() {
		gexec.CleanupBuildArtifacts()
	})
})

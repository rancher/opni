package rules_test

import (
	"context"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/rules"
)

var _ = Describe("Filesystem Rule Group Discovery", Label("unit"), func() {
	testGroup1 := []byte(`groups: [{name: test1}]`)
	testGroup2 := []byte(`groups: [{name: test2}, {name: test3}]`)
	testGroup3 := []byte(`groups: [{name: test4}, {name: test5}, {name: test6}]`)
	testFS := fstest.MapFS{
		"foo/bar/baz.yaml": &fstest.MapFile{Data: testGroup1},
		"test.yaml":        &fstest.MapFile{Data: testGroup2},
		"a/b/c/d/e/f.yaml": &fstest.MapFile{Data: testGroup3},
	}
	It("should find rule groups by filename", func() {
		finder := rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{"foo/bar/baz.yaml"},
		}, rules.WithFS(testFS))

		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(1))
		Expect(groups[0].Name).To(Equal("test1"))
	})
	It("should find rule groups by glob pattern", func() {
		finder := rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{"*.yaml"},
		}, rules.WithFS(testFS))

		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(2))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
		}).To(ContainElements("test2", "test3"))
	})
	It("should find rule groups by glob pattern and filename", func() {
		finder := rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{"*.yaml", "foo/bar/baz.yaml"},
		}, rules.WithFS(testFS))

		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(3))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
			groups[2].Name,
		}).To(ContainElements("test1", "test2", "test3"))
	})
	It("should find rule groups using a double-star pattern", func() {
		finder := rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{"**/f.yaml"},
		}, rules.WithFS(testFS))

		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(3))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
			groups[2].Name,
		}).To(ContainElements("test4", "test5", "test6"))

		finder = rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{"**/*.yaml"},
		}, rules.WithFS(testFS))

		groups, err = finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(6))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
			groups[2].Name,
			groups[3].Name,
			groups[4].Name,
			groups[5].Name,
		}).To(ContainElements("test1", "test2", "test3", "test4", "test5", "test6"))
	})
	It("should handle errors", func() {
		errFS := fstest.MapFS{
			// this technically only breaks because of MapFS, but it triggers the
			// read error code path anyway.
			"read-error.yaml/": &fstest.MapFile{
				Data: []byte("groups: [{name: test1}]"),
			},
			"invalid-format.yaml": &fstest.MapFile{Data: []byte("format = invalid;")},
		}
		finder := rules.NewFilesystemRuleFinder(&v1beta1.FilesystemRulesSpec{
			PathExpressions: []string{
				"/[",
				"*.yaml",
			},
		}, rules.WithFS(errFS))

		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(BeEmpty())
	})
})

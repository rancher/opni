package capabilities_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/test"
)

var lg = test.Log

var _ = Describe("Store", Ordered, Label("unit"), func() {
	var store capabilities.BackendStore
	var ctrl *gomock.Controller

	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})

	BeforeEach(func() {
		store = capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{
			Address: "localhost",
		}, lg)
	})

	When("creating a new store", func() {
		It("should be empty", func() {
			Expect(store.List()).To(BeEmpty())
		})
	})
	When("adding items to the store", func() {
		It("should be able to list and get items", func() {
			backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
				Name:              "capability1",
				CanInstall:        true,
				InstallerTemplate: "foo",
			})
			backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
				Name:              "capability2",
				CanInstall:        true,
				InstallerTemplate: "bar",
			})
			Expect(store.Add("capability1", backend1)).To(Succeed())
			Expect(store.List()).To(ContainElements("capability1"))
			Expect(store.Add("capability2", backend2)).To(Succeed())
			Expect(store.List()).To(ContainElements("capability1", "capability2"))

			Expect(store.Get("capability1")).To(Equal(backend1))
			Expect(store.Get("capability2")).To(Equal(backend2))
		})
		It("should return an error if the item already exists", func() {
			backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
				Name:              "capability1",
				CanInstall:        true,
				InstallerTemplate: "foo",
			})
			Expect(store.Add("capability1", backend1)).To(Succeed())
			Expect(store.Add("capability1", backend1)).To(MatchError(capabilities.ErrBackendAlreadyExists))
		})
	})
	When("getting items from the store", func() {
		It("should return an error if the item does not exist", func() {
			_, err := store.Get("capability1")
			Expect(err).To(MatchError(capabilities.ErrBackendNotFound))
		})
	})
	When("rendering install commands", func() {
		It("should allow rendering installer templates for each backend", func() {
			backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
				Name:              "capability1",
				CanInstall:        true,
				InstallerTemplate: "foo {{ .Token }} {{ .Pin }} [{{ .Address }}]",
			})
			backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
				Name:              "capability2",
				CanInstall:        true,
				InstallerTemplate: "{{ .Address }} {{ .Token | quote }} {{ .Pin | title }}",
			})
			Expect(store.Add("capability1", backend1)).To(Succeed())
			Expect(store.Add("capability2", backend2)).To(Succeed())

			installer1, err := store.RenderInstaller("capability1", capabilities.UserInstallerTemplateSpec{
				Token: "foo",
				Pin:   "bar",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(installer1).To(Equal("foo foo bar [localhost]"))

			installer2, err := store.RenderInstaller("capability2", capabilities.UserInstallerTemplateSpec{
				Token: "foo",
				Pin:   "bar",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(installer2).To(Equal("localhost \"foo\" Bar"))
		})

		DescribeTable("custom arg rendering",
			func(template string, expected string) {
				Expect(store.Add("capability", test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
					Name:              "capability1",
					CanInstall:        true,
					InstallerTemplate: template,
				}))).To(Succeed())

				installer, err := store.RenderInstaller("capability", capabilities.UserInstallerTemplateSpec{})
				Expect(err).NotTo(HaveOccurred())
				Expect(installer).To(Equal(expected))
			},
			Entry(nil, `{{ arg "input" "test" }}`, `%{"kind":"input","input":{"label":"test"}}%`),
			Entry(nil, `{{ arg "input" "test" "+required" "+omitEmpty" "+default:foo" "+format:{{value}}" }}`, `%{"kind":"input","input":{"label":"test"},"options":[{"name":"required"},{"name":"omitEmpty"},{"name":"default","value":"foo"},{"name":"format","value":"{{value}}"}]}%`),
			Entry(nil, `{{ arg "select" "test" "item1" "item2" "item3" }}`, `%{"kind":"select","select":{"label":"test","items":["item1","item2","item3"]}}%`),
			Entry(nil, `{{ arg "select" "test" "item1" "item2" "item3" "+required" "+omitEmpty" "+default:foo" "+format:{{value}}" }}`, `%{"kind":"select","select":{"label":"test","items":["item1","item2","item3"]},"options":[{"name":"required"},{"name":"omitEmpty"},{"name":"default","value":"foo"},{"name":"format","value":"{{value}}"}]}%`),
			Entry(nil, `{{ arg "toggle" "test" "+default:true" }}`, `%{"kind":"toggle","toggle":{"label":"test"},"options":[{"name":"default","value":"true"}]}%`),
		)

		When("the backend does not exist", func() {
			It("should return an error", func() {
				_, err := store.RenderInstaller("capability1", capabilities.UserInstallerTemplateSpec{
					Token: "foo",
					Pin:   "bar",
				})
				Expect(err).To(MatchError(capabilities.ErrBackendNotFound))
			})
		})
	})
	It("should check if backends can be installed", func() {
		backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
		})
		backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability2",
			CanInstall: false,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())
		Expect(store.Add("capability2", backend2)).To(Succeed())

		errText := "cannot install capability \"capability2\": test error"
		Expect(store.CanInstall("capability1")).To(Succeed())
		Expect(store.CanInstall("capability2")).To(MatchError(errText))
		Expect(store.CanInstall("capability1", "capability2")).To(MatchError(errText))
		Expect(store.CanInstall("capability3")).To(MatchError(capabilities.ErrUnknownCapability))
	})

	It("should install capabilities", func() {
		backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
		})
		backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability2",
			CanInstall: false,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())
		Expect(store.Add("capability2", backend2)).To(Succeed())

		store.InstallCapabilities(&corev1.Reference{}, "capability1", "capability2")
	})
	It("should uninstall capabilities", func() {
		backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
		})
		backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability2",
			CanInstall: false,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())
		Expect(store.Add("capability2", backend2)).To(Succeed())

		store.UninstallCapabilities(&corev1.Reference{}, "capability1", "capability2")
	})
})

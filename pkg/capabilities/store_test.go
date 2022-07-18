package capabilities_test

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/test"
)

var lg = test.Log

var _ = Describe("Store", Ordered, Label("unit"), func() {
	var store capabilities.BackendStore
	var ctrl *gomock.Controller
	var mockClusterStore storage.ClusterStore

	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})

	BeforeEach(func() {
		store = capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{
			Address: "localhost",
		}, lg)
		mockClusterStore = test.NewTestClusterStore(ctrl)
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

				installer, err := store.RenderInstaller("capability", capabilities.UserInstallerTemplateSpec{
					Token: "foo",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(installer).To(Equal(expected))
			},
			Entry(nil, `{{ arg "input" "test" }}`, `%{"kind":"input","input":{"label":"test"}}%`),
			Entry(nil, `{{ arg "input" "test" "+required" "+omitEmpty" "+default:foo" "+format:{{value}}" }}`, `%{"kind":"input","input":{"label":"test"},"options":[{"name":"required"},{"name":"omitEmpty"},{"name":"default","value":"foo"},{"name":"format","value":"{{value}}"}]}%`),
			Entry(nil, `{{ arg "select" "test" "item1" "item2" "item3" }}`, `%{"kind":"select","select":{"label":"test","items":["item1","item2","item3"]}}%`),
			Entry(nil, `{{ arg "select" "test" "item1" "item2" "item3" "+required" "+omitEmpty" "+default:foo" "+format:{{value}}" }}`, `%{"kind":"select","select":{"label":"test","items":["item1","item2","item3"]},"options":[{"name":"required"},{"name":"omitEmpty"},{"name":"default","value":"foo"},{"name":"format","value":"{{value}}"}]}%`),
			Entry(nil, `{{ arg "toggle" "test" "+default:true" }}`, `%{"kind":"toggle","toggle":{"label":"test"},"options":[{"name":"default","value":"true"}]}%`),
			Entry(nil, `{{ arg "toggle" "test" "+omitEmpty" "+default:{{ .Token }}" }}`, `%{"kind":"toggle","toggle":{"label":"test"},"options":[{"name":"omitEmpty"},{"name":"default","value":"foo"}]}%`),
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

		c1, err := store.Get("capability1")
		Expect(err).NotTo(HaveOccurred())
		c2, err := store.Get("capability2")
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Get("capability3")
		Expect(err).To(MatchError(capabilities.ErrBackendNotFound))

		_, err = c1.CanInstall(context.Background(), nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = c2.CanInstall(context.Background(), nil)
		Expect(err).To(MatchError("test error"))
	})

	It("should install capabilities", func() {
		cluster := &corev1.Cluster{
			Id: "cluster1",
		}
		mockClusterStore.CreateCluster(context.Background(), cluster)
		backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
			Storage:    mockClusterStore,
		})
		backend2 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability2",
			CanInstall: false,
			Storage:    mockClusterStore,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())
		Expect(store.Add("capability2", backend2)).To(Succeed())

		c1, err := store.Get("capability1")
		Expect(err).NotTo(HaveOccurred())
		c2, err := store.Get("capability2")
		Expect(err).NotTo(HaveOccurred())
		_, err = c1.Install(context.Background(), &v1.InstallRequest{
			Cluster: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = c2.Install(context.Background(), &v1.InstallRequest{
			Cluster: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())
	})
	It("should uninstall capabilities", func() {
		cluster := &corev1.Cluster{
			Id: "cluster1",
		}
		mockClusterStore.CreateCluster(context.Background(), cluster)
		backend1 := test.NewTestCapabilityBackend(ctrl, &test.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
			Storage:    mockClusterStore,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())

		c1, err := store.Get("capability1")
		Expect(err).NotTo(HaveOccurred())
		_, err = c1.Install(context.Background(), &v1.InstallRequest{
			Cluster: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			c, err := mockClusterStore.GetCluster(context.Background(), cluster.Reference())
			if err != nil {
				return err
			}
			for _, cap := range c.GetCapabilities() {
				if cap.GetName() == "capability1" {
					return nil
				}
			}
			return fmt.Errorf("capability1 not found")
		})

		_, err = c1.Uninstall(context.Background(), &v1.UninstallRequest{
			Cluster: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() task.State {
			stat, err := c1.UninstallStatus(context.Background(), cluster.Reference())
			if err != nil {
				return task.StateUnknown
			}
			return stat.State
		}).Should(Equal(task.StateCompleted))

		Eventually(func() error {
			c, err := mockClusterStore.GetCluster(context.Background(), cluster.Reference())
			if err != nil {
				return err
			}
			for _, cap := range c.GetCapabilities() {
				if cap.GetName() == "capability1" {
					return fmt.Errorf("capability1 not deleted")
				}
			}
			return nil
		})
	})
})

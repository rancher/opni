package capabilities_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	mock_v1 "github.com/rancher/opni/pkg/test/mock/capability"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/test/testlog"
	"go.uber.org/mock/gomock"
)

var lg = testlog.Log

var _ = Describe("Store", Ordered, Label("unit"), func() {
	var store capabilities.BackendStore
	var ctrl *gomock.Controller
	var mockClusterStore storage.ClusterStore

	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
	})

	BeforeEach(func() {
		store = capabilities.NewBackendStore(lg)
		mockClusterStore = mock_storage.NewTestClusterStore(ctrl)
	})

	When("creating a new store", func() {
		It("should be empty", func() {
			Expect(store.List()).To(BeEmpty())
		})
	})
	When("adding items to the store", func() {
		It("should be able to list and get items", func() {
			backend1 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
				Name:              "capability1",
				CanInstall:        true,
				InstallerTemplate: "foo",
			})
			backend2 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
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
			backend1 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
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

	It("should install capabilities", func() {
		cluster := &corev1.Cluster{
			Id: "cluster1",
		}
		mockClusterStore.CreateCluster(context.Background(), cluster)
		backend1 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
			Storage:    mockClusterStore,
		})
		backend2 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
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
			Agent: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = c2.Install(context.Background(), &v1.InstallRequest{
			Agent: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())
	})
	It("should uninstall capabilities", func() {
		cluster := &corev1.Cluster{
			Id: "cluster1",
		}
		mockClusterStore.CreateCluster(context.Background(), cluster)
		backend1 := mock_v1.NewTestCapabilityBackend(ctrl, &mock_v1.CapabilityInfo{
			Name:       "capability1",
			CanInstall: true,
			Storage:    mockClusterStore,
		})
		Expect(store.Add("capability1", backend1)).To(Succeed())

		c1, err := store.Get("capability1")
		Expect(err).NotTo(HaveOccurred())
		_, err = c1.Install(context.Background(), &v1.InstallRequest{
			Agent: cluster.Reference(),
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
			Agent: cluster.Reference(),
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() task.State {
			stat, err := c1.UninstallStatus(context.Background(), &v1.UninstallStatusRequest{Agent: cluster.Reference()})
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

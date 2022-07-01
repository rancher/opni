package management_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
)

var _ = Describe("Clusters", Ordered, Label("slow"), func() {
	var tv *testVars
	var capBackendStore capabilities.BackendStore
	BeforeAll(func() {
		capBackendStore = capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{}, test.Log)

		setupManagementServer(&tv, plugins.NoopLoader, management.WithCapabilitiesDataSource(testCapabilityDataSource{
			store: capBackendStore,
		}))()
	})

	It("should initially have no clusters", func() {
		clusters, err := tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusters.Items).To(BeEmpty())
	})
	events := make(chan *managementv1.WatchEvent, 1000)
	var streamCancel context.CancelFunc
	It("should handle watching create and delete events", func() {
		ctx, ca := context.WithCancel(context.Background())
		streamCancel = ca
		stream, err := tv.client.WatchClusters(ctx, &managementv1.WatchClustersRequest{
			KnownClusters: &corev1.ReferenceList{
				Items: []*corev1.Reference{},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		go func() {
			defer close(events)
			for {
				event, err := stream.Recv()
				if err != nil {
					return
				}
				events <- event
			}
		}()
	})
	It("should create clusters", func() {
		for x := 0; x < 3; x++ {
			ids := map[string]struct{}{}
			for i := 0; i < 10; i++ {
				id := uuid.NewString()
				ids[id] = struct{}{}
				err := tv.storageBackend.CreateCluster(context.Background(), &corev1.Cluster{
					Id: id,
					Metadata: &corev1.ClusterMetadata{
						Labels: map[string]string{
							"i": fmt.Sprint(i + (x * 10)),
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			}
			timeout := time.After(1100 * time.Millisecond)
			for i := 0; i < 10; i++ {
				select {
				case event := <-events:
					Expect(event.Type).To(Equal(managementv1.WatchEventType_Added))
					Expect(ids).To(HaveKey(event.Cluster.Id))
					cluster, err := tv.client.GetCluster(context.Background(), &corev1.Reference{
						Id: event.Cluster.Id,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(cluster.Metadata.Labels).To(HaveKey("i"))
					delete(ids, event.Cluster.Id)
				case <-timeout:
					Fail("timed out waiting for cluster create events")
				}
			}
			Expect(ids).To(BeEmpty())

			clusters, err := tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(HaveLen(10 * (x + 1)))
		}
	})
	It("should edit cluster labels", func() {
		cluster, err := tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchExpressions: []*corev1.LabelSelectorRequirement{
					{
						Key:      "i",
						Operator: string(corev1.LabelSelectorOpIn),
						Values:   []string{"20"},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster.Items).To(HaveLen(1))
		ref := cluster.Items[0].Reference()
		updated, err := tv.client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: ref,
			Labels: map[string]string{
				"i": "999",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Metadata.Labels).To(HaveKeyWithValue("i", "999"))

		updatedQueried, err := tv.client.GetCluster(context.Background(), ref)
		Expect(err).NotTo(HaveOccurred())
		Expect(updatedQueried.Metadata.Labels).To(HaveKeyWithValue("i", "999"))
	})
	It("should delete clusters", func() {
		clusters, err := tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusters.Items).To(HaveLen(30))
		ids := map[string]struct{}{}
		for _, cluster := range clusters.Items {
			ids[cluster.Id] = struct{}{}
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer GinkgoRecover()

			for event := range events {
				Expect(event.Type).To(Equal(managementv1.WatchEventType_Deleted))
				Expect(ids).To(HaveKey(event.Cluster.Id))
				delete(ids, event.Cluster.Id)

				_, err := tv.client.GetCluster(context.Background(), &corev1.Reference{
					Id: event.Cluster.Id,
				})
				Expect(util.StatusCode(err)).To(Equal(codes.NotFound))
				if len(ids) == 0 {
					return
				}
			}
		}()
		for _, cluster := range clusters.Items {
			_, err := tv.client.DeleteCluster(context.Background(), &corev1.Reference{
				Id: cluster.Id,
			})
			Expect(err).NotTo(HaveOccurred())
			// watch events should be batched every second, wait 4 seconds in total
			// for all events to be received
			time.Sleep(100 * time.Millisecond)
		}
		Eventually(done).Should(BeClosed())

		clusters, err = tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusters.Items).To(BeEmpty())
		streamCancel()
	})
	When("attempting to edit a nonexistent cluster", func() {
		It("should error", func() {
			_, err := tv.client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
				Cluster: &corev1.Reference{
					Id: "nonexistent",
				},
				Labels: map[string]string{},
			})
			Expect(util.StatusCode(err)).To(Equal(codes.NotFound))
		})
	})
	When("attempting to delete a nonexistent cluster", func() {
		It("should error", func() {
			_, err := tv.client.DeleteCluster(context.Background(), &corev1.Reference{
				Id: "nonexistent",
			})
			Expect(util.StatusCode(err)).To(Equal(codes.NotFound))
		})
	})
	It("should handle validation errors", func() {
		_, err := tv.client.ListClusters(context.Background(), &managementv1.ListClustersRequest{
			MatchLabels: &corev1.LabelSelector{
				MatchLabels: map[string]string{
					"\\": "bar",
				},
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(validation.ErrInvalidLabelName.Error()))

		_, err = tv.client.GetCluster(context.Background(), &corev1.Reference{
			Id: "\\",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(validation.ErrInvalidID.Error()))

		_, err = tv.client.EditCluster(context.Background(), &managementv1.EditClusterRequest{
			Cluster: &corev1.Reference{
				Id: "\\",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(validation.ErrInvalidID.Error()))

		_, err = tv.client.DeleteCluster(context.Background(), &corev1.Reference{
			Id: "\\",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(validation.ErrInvalidID.Error()))

		stream, err := tv.client.WatchClusters(context.Background(), &managementv1.WatchClustersRequest{
			KnownClusters: &corev1.ReferenceList{
				Items: []*corev1.Reference{
					{
						Id: "\\",
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = stream.Recv()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(validation.ErrInvalidID.Error()))

		stream, err = tv.client.WatchClusters(context.Background(), &managementv1.WatchClustersRequest{
			KnownClusters: &corev1.ReferenceList{
				Items: []*corev1.Reference{
					{
						Id: "nonexistent",
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = stream.Recv()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(storage.ErrNotFound.Error()))
	})
})

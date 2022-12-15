package conformance

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

func newIdWithLine() string {
	_, _, line, ok := runtime.Caller(1)
	if !ok {
		panic("failed to get caller")
	}
	return fmt.Sprintf("%s-%d", uuid.New().String(), line)
}

func line() string {
	_, _, line, ok := runtime.Caller(1)
	if !ok {
		panic("failed to get caller")
	}
	return strconv.Itoa(line)
}

func ClusterStoreTestSuite[T storage.ClusterStore](
	tsF future.Future[T],
) func() {
	return func() {
		var ts T
		BeforeAll(func() {
			ts = tsF.Get()
		})
		It("should initially have no clusters", func() {
			clusters, err := ts.ListClusters(context.Background(), &corev1.LabelSelector{}, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(BeEmpty())
		})
		When("creating a cluster", func() {
			It("should be retrievable", func() {
				cluster := &corev1.Cluster{
					Id: "foo",
					Metadata: &corev1.ClusterMetadata{
						Labels: map[string]string{
							"foo": "bar",
							"bar": "baz",
						},
						Capabilities: []*corev1.ClusterCapability{
							{
								Name: "foo",
							},
						},
					},
				}
				createTimestampLowerBound := time.Now()
				Eventually(func() error {
					return ts.CreateCluster(context.Background(), cluster)
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
				createTimestampUpperBound := time.Now()
				cluster, err := ts.GetCluster(context.Background(), cluster.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster).NotTo(BeNil())
				Expect(cluster.Id).To(Equal("foo"))
				Expect(cluster.GetCreationTimestamp().Unix()).To(And(
					BeNumerically(">=", createTimestampLowerBound.Unix()),
					BeNumerically("<=", createTimestampUpperBound.Unix()),
				))
				Expect(cluster.GetCreationTimestamp().Nanosecond()).To(Equal(0))
				Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("bar", "baz"))
				Expect(cluster.Metadata.Capabilities).To(HaveLen(1))
				Expect(cluster.Metadata.Capabilities[0].Name).To(Equal("foo"))
			})
			It("should appear in the list of clusters", func() {
				clusters, err := ts.ListClusters(context.Background(), &corev1.LabelSelector{}, 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(clusters.Items).To(HaveLen(1))
				Expect(clusters.Items[0].GetId()).To(Equal("foo"))
				Expect(clusters.Items[0].GetMetadata().Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(clusters.Items[0].GetMetadata().Labels).To(HaveKeyWithValue("bar", "baz"))
				Expect(clusters.Items[0].GetMetadata().Capabilities).To(HaveLen(1))
				Expect(clusters.Items[0].GetMetadata().Capabilities[0].Name).To(Equal("foo"))
			})
			It("should fail if the cluster already exists", func() {
				cluster := &corev1.Cluster{
					Id: newIdWithLine(),
					Metadata: &corev1.ClusterMetadata{
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				}
				err := ts.CreateCluster(context.Background(), cluster)
				Expect(err).NotTo(HaveOccurred())

				err = ts.CreateCluster(context.Background(), cluster)
				Expect(err).To(MatchError(storage.ErrAlreadyExists))
			})
		})
		It("should list clusters with a label selector", func() {
			create := func(labels map[string]string) *corev1.Cluster {
				cluster := &corev1.Cluster{
					Id: newIdWithLine(),
					Metadata: &corev1.ClusterMetadata{
						Labels: labels,
					},
				}
				err := ts.CreateCluster(context.Background(), cluster)
				Expect(err).NotTo(HaveOccurred())
				return cluster
			}
			for i := 0; i < 5; i++ {
				create(map[string]string{"testing": "foo"})
			}
			for i := 0; i < 5; i++ {
				create(map[string]string{"testing": "bar"})
			}
			sel := &corev1.ClusterSelector{
				LabelSelector: &corev1.LabelSelector{
					MatchLabels: map[string]string{
						"testing": "foo",
					},
				},
			}
			clusters, err := ts.ListClusters(context.Background(), sel.LabelSelector, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(HaveLen(5))
			for _, cluster := range clusters.Items {
				Expect(storage.NewSelectorPredicate(sel)(cluster)).To(BeTrue())
			}
			sel = &corev1.ClusterSelector{
				LabelSelector: &corev1.LabelSelector{
					MatchLabels: map[string]string{
						"testing": "bar",
					},
				},
			}
			clusters, err = ts.ListClusters(context.Background(), sel.LabelSelector, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(HaveLen(5))
			for _, cluster := range clusters.Items {
				Expect(storage.NewSelectorPredicate(sel)(cluster)).To(BeTrue())
			}
		})
		It("should respect match options", func() {
			clusters, err := ts.ListClusters(context.Background(), nil, corev1.MatchOptions_EmptySelectorMatchesNone)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(BeEmpty())
			clusters, err = ts.ListClusters(context.Background(), &corev1.LabelSelector{}, corev1.MatchOptions_EmptySelectorMatchesNone)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(BeEmpty())
			clusters, err = ts.ListClusters(context.Background(), &corev1.LabelSelector{
				MatchLabels:      map[string]string{},
				MatchExpressions: []*corev1.LabelSelectorRequirement{},
			}, corev1.MatchOptions_EmptySelectorMatchesNone)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters.Items).To(BeEmpty())
		})
		It("should delete clusters", func() {
			all, err := ts.ListClusters(context.Background(), nil, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(all.Items).NotTo(BeEmpty())

			for _, cluster := range all.Items {
				err := ts.DeleteCluster(context.Background(), cluster.Reference())
				Expect(err).NotTo(HaveOccurred())
			}

			all, err = ts.ListClusters(context.Background(), nil, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(all.Items).To(BeEmpty())
		})
		It("should be able to edit labels", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			cluster, err = ts.GetCluster(context.Background(), cluster.Reference())
			Expect(err).NotTo(HaveOccurred())

			prevVersion := cluster.GetResourceVersion()
			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(), func(c *corev1.Cluster) {
				c.Metadata.Labels["foo"] = "baz-180"
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.GetResourceVersion()).NotTo(BeEmpty())
			Expect(cluster.GetResourceVersion()).NotTo(Equal(prevVersion))
			if integer, err := strconv.Atoi(cluster.GetResourceVersion()); err == nil {
				Expect(integer).To(BeNumerically(">", util.Must(strconv.Atoi(prevVersion))))
			}

			cluster, err = ts.GetCluster(context.Background(), cluster.Reference())
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("foo", "baz-180"))
		})
		It("should be able to add capabilities", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())
			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(),
				storage.NewAddCapabilityMutator[*corev1.Cluster](&corev1.ClusterCapability{
					Name: "foo",
				}),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.GetCapabilities()).To(HaveLen(1))
			Expect(cluster.GetCapabilities()[0].Name).To(Equal("foo"))
		})
		It("should be able to edit multiple properties at once", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(),
				storage.NewCompositeMutator(
					storage.NewAddCapabilityMutator[*corev1.Cluster](&corev1.ClusterCapability{
						Name: "foo",
					}),
					func(c *corev1.Cluster) {
						c.Metadata.Labels["foo"] = "baz-231"
					},
				),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.GetCapabilities()).To(HaveLen(1))
			Expect(cluster.GetCapabilities()[0].Name).To(Equal("foo"))
			Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("foo", "baz-231"))
		})
		It("should handle multiple concurrent update requests on the same resource", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"value": "0",
					},
				},
			}
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			wg := sync.WaitGroup{}
			start := make(chan struct{})
			count := testutil.IfCI(5).Else(10)
			for i := 0; i < count; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					ts.UpdateCluster(context.Background(), cluster.Reference(),
						func(c *corev1.Cluster) {
							c.Metadata.Labels["value"] = strconv.Itoa(util.Must(strconv.Atoi(c.Metadata.Labels["value"])) + 1)
						},
					)
				}()
			}
			close(start)
			wg.Wait()

			cluster, err = ts.GetCluster(context.Background(), cluster.Reference())
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("value", strconv.Itoa(count)))
		})
		It("should watch for changes to a cluster", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			By("creating a cluster")
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			ctx, ca := context.WithCancel(context.Background())
			defer ca()
			By("starting a watch")
			wc, err := ts.WatchCluster(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring no updates are received")
			Consistently(wc).ShouldNot(Receive())

			By("updating the cluster")
			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(),
				func(c *corev1.Cluster) {
					c.Metadata.Labels["foo"] = "baz-299"
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring an Update update is received")
			select {
			case event := <-wc:
				Expect(event.EventType).To(Equal(storage.WatchEventUpdate))
				Expect(event.Previous.Metadata.Labels).To(Equal(map[string]string{
					"foo": "bar",
				}))
				Expect(event.Current.Metadata.Labels).To(Equal(map[string]string{
					"foo": "baz-299",
				}))
			case <-time.After(5 * time.Second):
				Fail("timed out waiting for watch event")
			}

			By("ensuring no more updates are received")
			Consistently(wc, 1*time.Second, 100*time.Millisecond).ShouldNot(Receive())

			By("deleting the cluster")
			err = ts.DeleteCluster(context.Background(), cluster.Reference())
			Expect(err).NotTo(HaveOccurred())

			By("ensuring a Delete update is received")
			select {
			case event := <-wc:
				Expect(event.EventType).To(Equal(storage.WatchEventDelete))
				Expect(event.Previous.Metadata.Labels).To(Equal(map[string]string{
					"foo": "baz-299",
				}))
				Expect(event.Current).To(BeNil())
			case <-time.After(5 * time.Second):
				Fail("timed out waiting for watch event")
			}
		})
		It("should immediately receive an update if the resource is modified before the watch is started", func() {
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			By("creating a cluster")
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			By("updating the cluster")
			_, err = ts.UpdateCluster(context.Background(), cluster.Reference(),
				func(c *corev1.Cluster) {
					c.Metadata.Labels["foo"] = "baz-353"
				},
			)
			Expect(err).NotTo(HaveOccurred())

			ctx, ca := context.WithCancel(context.Background())
			By("starting a watch with the previous version")
			wc, err := ts.WatchCluster(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring an Update update is received")
			select {
			case event := <-wc:
				Expect(event.EventType).To(Equal(storage.WatchEventUpdate))
				Expect(event.Previous.GetResourceVersion()).To(Equal(cluster.GetResourceVersion()))
				Expect(event.Current.GetResourceVersion()).NotTo(Equal(cluster.GetResourceVersion()))
				Expect(event.Previous.Metadata.Labels).To(Equal(map[string]string{
					"foo": "bar",
				}))
				Expect(event.Current.Metadata.Labels).To(Equal(map[string]string{
					"foo": "baz-353",
				}))
			case <-time.After(5 * time.Second):
				Fail("timed out waiting for watch event")
			}
			ca()
			Consistently(wc, 2*time.Second, 100*time.Millisecond).ShouldNot(Receive())
		})
		It("should watch all clusters", func() {
			ctx, ca := context.WithCancel(context.Background())
			defer ca()

			allClusters, err := ts.ListClusters(ctx, &corev1.LabelSelector{}, 0)
			Expect(err).NotTo(HaveOccurred())

			By("starting a watch on all clusters")
			wc, err := ts.WatchClusters(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring create events are received for all existing clusters")
			toRecv := make(map[string]*corev1.Cluster)
			for _, cluster := range allClusters.Items {
				toRecv[cluster.Id] = cluster
			}
			for i := 0; i < len(allClusters.Items); i++ {
				event := <-wc
				Expect(event.EventType).To(Equal(storage.WatchEventCreate))
				Expect(event.Previous).To(BeNil())
				Expect(event.Current).NotTo(BeNil())
				id := event.Current.Id
				Expect(toRecv).To(HaveKey(id))
				Expect(event.Current.Metadata.Labels).To(Equal(toRecv[id].Metadata.Labels))
				delete(toRecv, id)
			}
			Expect(toRecv).To(BeEmpty())

			By("ensuring no more events are received")
			select {
			case event := <-wc:
				Fail("unexpected event received:" + fmt.Sprint(event))
			case <-time.After(1 * time.Second):
			}

			By("creating a cluster")
			cluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err = ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			By("creating another cluster")
			cluster2 := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err = ts.CreateCluster(context.Background(), cluster2)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring Create events are received for both clusters")
			var event storage.WatchEvent[*corev1.Cluster]
			var ids []string
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventCreate))
			Expect(event.Previous).To(BeNil())
			Expect(event.Current).NotTo(BeNil())
			ids = append(ids, event.Current.Id)

			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventCreate))
			Expect(event.Previous).To(BeNil())
			Expect(event.Current).NotTo(BeNil())
			ids = append(ids, event.Current.Id)

			Expect(ids).To(ConsistOf(cluster.Id, cluster2.Id))

			By("updating the first cluster")
			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(),
				func(c *corev1.Cluster) {
					c.Metadata.Labels["foo"] = "baz-458"
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring an Update event is received for the first cluster")
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventUpdate))
			Expect(event.Previous.Id).To(Equal(cluster.Id))
			Expect(event.Current.Id).To(Equal(cluster.Id))
			Expect(event.Current.Metadata.Labels).To(Equal(map[string]string{
				"foo": "baz-458",
			}))

			By("deleting the second cluster")
			err = ts.DeleteCluster(context.Background(), cluster2.Reference())
			Expect(err).NotTo(HaveOccurred())

			By("ensuring a Delete event is received for the second cluster")
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventDelete))
			Expect(event.Previous.Id).To(Equal(cluster2.Id))
			Expect(event.Current).To(BeNil())
		})
		It("should watch all clusters starting from a set of already-known clusters", func() {
			By("adding all existing clusters to the known set")
			allClusters, err := ts.ListClusters(context.Background(), &corev1.LabelSelector{}, 0)
			Expect(err).NotTo(HaveOccurred())

			By("creating a cluster which will not be known")
			newCluster := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err = ts.CreateCluster(context.Background(), newCluster)
			Expect(err).NotTo(HaveOccurred())

			By("updating one of the existing clusters but keeping the previous version in the known set")
			randomUpdatedLabelValue := newIdWithLine()
			_, err = ts.UpdateCluster(context.Background(), allClusters.Items[0].Reference(),
				func(c *corev1.Cluster) {
					c.Metadata.Labels["foo"] = randomUpdatedLabelValue
				},
			)

			ctx, ca := context.WithCancel(context.Background())
			defer ca()
			By("starting a watch on all clusters")
			wc, err := ts.WatchClusters(ctx, allClusters.Items)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring one create and one update event are received")
			events := make(chan storage.WatchEvent[*corev1.Cluster], 2)

			for i := 0; i < 2; i++ {
				select {
				case e := <-wc:
					events <- e
				case <-time.After(time.Second):
					Fail("timed out waiting for events")
				}
			}

			e1, e2 := <-events, <-events
			Expect(e1.EventType).NotTo(Equal(e2.EventType))
			var create, update storage.WatchEvent[*corev1.Cluster]
			if e1.EventType == storage.WatchEventCreate {
				create, update = e1, e2
			} else {
				create, update = e2, e1
			}
			Expect(create.EventType).To(Equal(storage.WatchEventCreate))
			Expect(create.Previous).To(BeNil())
			Expect(create.Current).NotTo(BeNil())
			Expect(create.Current.Id).To(Equal(newCluster.Id))

			Expect(update.EventType).To(Equal(storage.WatchEventUpdate))
			Expect(update.Previous.Id).To(Equal(allClusters.Items[0].Id))
			Expect(update.Current.Id).To(Equal(allClusters.Items[0].Id))
			Expect(update.Current.Metadata.Labels).To(HaveKeyWithValue("foo", randomUpdatedLabelValue))

			By("ensuring no more events are received")
			Consistently(wc).ShouldNot(Receive())

			By("creating another cluster")
			cluster2 := &corev1.Cluster{
				Id: newIdWithLine(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err = ts.CreateCluster(context.Background(), cluster2)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring a Create event is received for the second cluster")
			var event storage.WatchEvent[*corev1.Cluster]
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventCreate))
			Expect(event.Previous).To(BeNil())
			Expect(event.Current).NotTo(BeNil())
			Expect(event.Current.Id).To(Equal(cluster2.Id))

			By("updating the first cluster")
			newCluster, err = ts.UpdateCluster(context.Background(), newCluster.Reference(),
				func(c *corev1.Cluster) {
					c.Metadata.Labels["foo"] = "baz-571"
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring an Update event is received for the first cluster")
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventUpdate))
			Expect(event.Previous.Id).To(Equal(newCluster.Id))
			Expect(event.Current.Id).To(Equal(newCluster.Id))
			Expect(event.Current.Metadata.Labels).To(Equal(map[string]string{
				"foo": "baz-571",
			}))

			By("deleting the second cluster")
			err = ts.DeleteCluster(context.Background(), cluster2.Reference())
			Expect(err).NotTo(HaveOccurred())

			By("ensuring a Delete event is received for the second cluster")
			Eventually(wc).Should(Receive(&event))
			Expect(event.EventType).To(Equal(storage.WatchEventDelete))
			Expect(event.Previous.Id).To(Equal(cluster2.Id))
			Expect(event.Current).To(BeNil())
		})
		When("deleting a cluster", func() {
			It("should fail if the cluster does not exist", func() {
				err := ts.DeleteCluster(context.Background(), &corev1.Reference{
					Id: newIdWithLine(),
				})
				Expect(err).To(MatchError(storage.ErrNotFound))
			})
		})
	}
}

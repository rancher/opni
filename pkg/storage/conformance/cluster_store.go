package conformance

import (
	"context"
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

func ClusterStoreTestSuite[T storage.ClusterStore](
	tsF future.Future[T],
	errCtrlF future.Future[ErrorController],
) func() {
	return func() {
		var ts T
		var errCtrl ErrorController
		BeforeAll(func() {
			ts = tsF.Get()
			errCtrl = errCtrlF.Get()
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
				Eventually(func() error {
					return ts.CreateCluster(context.Background(), cluster)
				}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
				cluster, err := ts.GetCluster(context.Background(), cluster.Reference())
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster).NotTo(BeNil())
				Expect(cluster.Id).To(Equal("foo"))
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
		})
		It("should list clusters with a label selector", func() {
			create := func(labels map[string]string) *corev1.Cluster {
				cluster := &corev1.Cluster{
					Id: uuid.NewString(),
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
			sel := storage.ClusterSelector{
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
				Expect(sel.Predicate()(cluster)).To(BeTrue())
			}
			sel = storage.ClusterSelector{
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
				Expect(sel.Predicate()(cluster)).To(BeTrue())
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
				Id: uuid.NewString(),
				Metadata: &corev1.ClusterMetadata{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			}
			err := ts.CreateCluster(context.Background(), cluster)
			Expect(err).NotTo(HaveOccurred())

			cluster, err = ts.UpdateCluster(context.Background(), cluster.Reference(), func(c *corev1.Cluster) {
				c.Metadata.Labels["foo"] = "baz"
			})
			Expect(err).NotTo(HaveOccurred())

			cluster, err = ts.GetCluster(context.Background(), cluster.Reference())
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("foo", "baz"))
		})
		It("should be able to add capabilities", func() {
			cluster := &corev1.Cluster{
				Id: uuid.NewString(),
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
				Id: uuid.NewString(),
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
						c.Metadata.Labels["foo"] = "baz"
					},
				),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.GetCapabilities()).To(HaveLen(1))
			Expect(cluster.GetCapabilities()[0].Name).To(Equal("foo"))
			Expect(cluster.Metadata.Labels).To(HaveKeyWithValue("foo", "baz"))
		})
		It("should handle multiple concurrent update requests on the same resource", func() {
			cluster := &corev1.Cluster{
				Id: uuid.NewString(),
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
		Context("error handling", func() {
			if runtime.GOOS != "linux" {
				Skip("skipping tests on non-linux OS")
			}
			It("should handle errors when creating clusters", func() {
				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					err := ts.CreateCluster(context.Background(), &corev1.Cluster{Id: uuid.NewString()})
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when getting clusters", func() {
				_, err := ts.GetCluster(context.Background(), &corev1.Reference{
					Id: uuid.NewString(),
				})
				Expect(err).To(HaveOccurred())

				cluster := &corev1.Cluster{Id: uuid.NewString()}
				err = ts.CreateCluster(context.Background(), cluster)
				Expect(err).NotTo(HaveOccurred())

				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err = ts.GetCluster(context.Background(), cluster.Reference())
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when listing clusters", func() {
				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err := ts.ListClusters(context.Background(), nil, 0)
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when updating clusters", func() {
				_, err := ts.UpdateCluster(context.Background(), &corev1.Reference{
					Id: uuid.NewString(),
				}, func(c *corev1.Cluster) {})
				Expect(err).To(HaveOccurred())

				cluster := &corev1.Cluster{Id: uuid.NewString()}
				err = ts.CreateCluster(context.Background(), cluster)
				Expect(err).NotTo(HaveOccurred())

				errCtrl.EnableErrors()
				defer errCtrl.DisableErrors()
				Eventually(func() error {
					_, err = ts.UpdateCluster(context.Background(), cluster.Reference(), func(c *corev1.Cluster) {
						if c.Metadata == nil {
							c.Metadata = &corev1.ClusterMetadata{}
						}
						if c.Metadata.Labels == nil {
							c.Metadata.Labels = map[string]string{}
						}
						c.Metadata.Labels["foo"] = "bar"
					})
					return err
				}).Should(HaveOccurred())
			})

			It("should handle errors when deleting clusters", func() {
				err := ts.DeleteCluster(context.Background(), &corev1.Reference{
					Id: uuid.NewString(),
				})
				Expect(err).To(HaveOccurred())
			})
		})
	}
}

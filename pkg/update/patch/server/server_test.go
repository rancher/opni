package server_test

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/test/memfs"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/noop"
	"github.com/rancher/opni/pkg/update/patch"
	"github.com/rancher/opni/pkg/update/patch/server"
	"github.com/rancher/opni/pkg/urn"
	"github.com/rancher/opni/pkg/util/streams"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Filesystem Sync Server (zstd)", Ordered, Label("unit", "slow"), FilesystemSyncServerTestSuite(zstdPatcher))
var _ = Describe("Filesystem Sync Server (bsdiff)", Ordered, Label("unit", "slow"), FilesystemSyncServerTestSuite(bsdiffPatcher))

func FilesystemSyncServerTestSuite(patcher patch.BinaryPatcher) func() {
	return func() {
		var srv *server.FilesystemPluginSyncServer
		var updateSrv *update.UpdateServer
		fsys := afero.Afero{Fs: memfs.NewModeAwareMemFs()}
		tmpDir := "/tmp/test"
		fsys.MkdirAll(tmpDir, 0755)

		var agentManifest *controlv1.UpdateManifest
		var srvManifestV1 *controlv1.UpdateManifest
		var srvManifestV2 *controlv1.UpdateManifest

		cacheSpec := &configv1.CacheSpec{
			Backend: configv1.CacheBackend_Filesystem.Enum(),
			Filesystem: &configv1.FilesystemCacheSpec{
				Dir: lo.ToPtr(filepath.Join(tmpDir, "cache")),
			},
		}
		newServer := func() (*server.FilesystemPluginSyncServer, error) {
			return server.NewFilesystemPluginSyncServer(context.Background(), cacheSpec, patcher, testlog.Log, server.WithFs(fsys))
		}
		newUpdateServer := func(s *server.FilesystemPluginSyncServer) *update.UpdateServer {
			srv := update.NewUpdateServer(testlog.Log)
			agentSrv := noop.NewSyncServer(noop.WithAllowedTypes(urn.Agent))
			srv.RegisterUpdateHandler(agentSrv.Strategy(), agentSrv)
			agentManifest, _ = agentSrv.CalculateExpectedManifest(context.Background(), urn.Agent)
			srv.RegisterUpdateHandler(s.Strategy(), s)
			return srv
		}

		BeforeEach(func() {
			updateSrv = newUpdateServer(srv)
		})

		When("starting the filesystem sync server", func() {
			It("should succeed", func() {
				fsys.Mkdir(filepath.Join(tmpDir, patch.PluginsDir), 0755)
				fsys.Mkdir(filepath.Join(tmpDir, "cache"), 0755)

				Expect(fsys.WriteFile(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test1"), testBinaries["test1"]["v1"], 0644)).To(Succeed())
				Expect(fsys.WriteFile(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test2"), testBinaries["test2"]["v1"], 0644)).To(Succeed())

				var err error
				srv, err = newServer()
				Expect(err).NotTo(HaveOccurred())
			})
			It("should have the correct manifest", func() {
				manifest, err := srv.CalculateExpectedManifest(context.Background(), urn.Plugin)
				Expect(err).NotTo(HaveOccurred())

				Expect(manifest.Items).To(HaveLen(2))
				Expect(manifest.Items[0].Package).To(Equal(test1Package))
				Expect(manifest.Items[1].Package).To(Equal(test2Package))
				Expect(manifest.Items[0].Digest).To(Equal(v1Manifest.Items[0].Metadata.Digest))
				Expect(manifest.Items[1].Digest).To(Equal(v1Manifest.Items[1].Metadata.Digest))
				Expect(manifest.Items[0].Path).To(Equal("plugin_test1"))
				Expect(manifest.Items[1].Path).To(Equal("plugin_test2"))

				srvManifestV1 = manifest
			})
			It("should have archived the plugins", func() {
				items, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PluginsDir))
				Expect(err).NotTo(HaveOccurred())

				Expect(items).To(HaveLen(2))
				names := []string{items[0].Name(), items[1].Name()}
				Expect(names).To(ConsistOf(v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest))
			})
		})

		When("restarting the server with updated plugins", func() {
			It("should succeed", func() {
				fsys.Remove(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test1"))
				fsys.Remove(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test2"))

				Expect(fsys.WriteFile(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test1"), testBinaries["test1"]["v2"], 0644)).To(Succeed())
				Expect(fsys.WriteFile(filepath.Join(tmpDir, patch.PluginsDir, "plugin_test2"), testBinaries["test2"]["v2"], 0644)).To(Succeed())

				var err error
				srv, err = newServer()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have the correct updated manifest", func() {
				manifest, err := srv.CalculateExpectedManifest(context.Background(), urn.Plugin)
				Expect(err).NotTo(HaveOccurred())

				Expect(manifest.Items).To(HaveLen(2))
				Expect(manifest.Items[0].Package).To(Equal(test1Package))
				Expect(manifest.Items[1].Package).To(Equal(test2Package))
				Expect(manifest.Items[0].Digest).To(Equal(v2Manifest.Items[0].Metadata.Digest))
				Expect(manifest.Items[1].Digest).To(Equal(v2Manifest.Items[1].Metadata.Digest))
				Expect(manifest.Items[0].Path).To(Equal("plugin_test1"))
				Expect(manifest.Items[1].Path).To(Equal("plugin_test2"))

				srvManifestV2 = manifest
			})

			It("should have archived the new versions", func() {
				items, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PluginsDir))
				Expect(err).NotTo(HaveOccurred())

				Expect(items).To(HaveLen(4))
				names := []string{
					items[0].Name(), items[1].Name(),
					items[2].Name(), items[3].Name(),
				}
				Expect(names).To(ConsistOf(
					v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest,
					v2Manifest.Items[0].Metadata.Digest, v2Manifest.Items[1].Metadata.Digest,
				))
			})
		})
		When("a client connects to sync their plugins", func() {
			var initialPatchResponse *controlv1.PatchList
			var initialCacheItems []fs.FileInfo
			When("the client has old v1 plugins", func() {
				It("should return patch operations", func() {
					patches, err := srv.CalculateUpdate(context.Background(), srvManifestV1)
					Expect(err).NotTo(HaveOccurred())

					Expect(patches.Items).To(HaveLen(2))
					Expect(patches.Items[0].Package).To(Equal(test1Package))
					Expect(patches.Items[0].Op).To(Equal(controlv1.PatchOp_Update))
					Expect(patches.Items[0].OldDigest).To(Equal(srvManifestV1.Items[0].Digest))
					Expect(patches.Items[0].NewDigest).To(Equal(srvManifestV2.Items[0].Digest))
					Expect(patches.Items[0].Path).To(Equal(srvManifestV1.Items[0].Path))
					Expect(patches.Items[0].Data).To(Equal(test1v1tov2Patch[patcher].Bytes()))

					Expect(patches.Items[1].Package).To(Equal(test2Package))
					Expect(patches.Items[1].Op).To(Equal(controlv1.PatchOp_Update))
					Expect(patches.Items[1].OldDigest).To(Equal(srvManifestV1.Items[1].Digest))
					Expect(patches.Items[1].NewDigest).To(Equal(srvManifestV2.Items[1].Digest))
					Expect(patches.Items[1].Path).To(Equal(srvManifestV1.Items[1].Path))
					Expect(patches.Items[1].Data).To(Equal(test2v1tov2Patch[patcher].Bytes()))

					initialPatchResponse = patches
				})
				It("should cache the patches", func() {
					items, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PatchesDir))
					Expect(err).NotTo(HaveOccurred())

					Expect(items).To(HaveLen(2))

					var patches [][]byte
					for _, item := range items {
						contents, err := fsys.ReadFile(filepath.Join(tmpDir, "cache", patch.PatchesDir, item.Name()))
						Expect(err).NotTo(HaveOccurred())
						patches = append(patches, contents)
					}

					Expect(patches).To(ConsistOf(
						test1v1tov2Patch[patcher].Bytes(),
						test2v1tov2Patch[patcher].Bytes(),
					))

					initialCacheItems = items
				})
			})
			When("another client connects", func() {
				It("should return patch operations using cached patches", func() {
					patches, err := srv.CalculateUpdate(context.Background(), srvManifestV1)
					Expect(err).NotTo(HaveOccurred())
					Expect(patches).To(testutil.ProtoEqual(initialPatchResponse))
				})
				It("should not modify the cache", func() {
					items, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PatchesDir))
					Expect(err).NotTo(HaveOccurred())

					Expect(items).To(Equal(initialCacheItems))
				})
			})
			When("the server is unable to provide patches for the request", func() {
				It("should return a create op with the full plugin contents", func() {
					patches, err := srv.CalculateUpdate(context.Background(), &controlv1.UpdateManifest{
						Items: []*controlv1.UpdateManifestEntry{
							{
								Package: test1Package,
								Digest:  "deadbeef",
								Path:    "plugin_test1",
							},
							{
								Package: test2Package,
								Digest:  "deadbeef",
								Path:    "plugin_test2",
							},
						},
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(patches.Items).To(HaveLen(2))
					Expect(patches.Items[0].Package).To(Equal(test1Package))
					Expect(patches.Items[0].Op).To(Equal(controlv1.PatchOp_Create))
					Expect(patches.Items[0].NewDigest).To(Equal(srvManifestV2.Items[0].Digest))
					Expect(patches.Items[0].Path).To(Equal("plugin_test1"))
					Expect(patches.Items[0].Data).To(Equal(testBinaries["test1"]["v2"]))

					Expect(patches.Items[1].Package).To(Equal(test2Package))
					Expect(patches.Items[1].Op).To(Equal(controlv1.PatchOp_Create))
					Expect(patches.Items[1].NewDigest).To(Equal(srvManifestV2.Items[1].Digest))
					Expect(patches.Items[1].Path).To(Equal("plugin_test2"))
					Expect(patches.Items[1].Data).To(Equal(testBinaries["test2"]["v2"]))
				})
				When("the server is unable to read a plugin on disk", func() {
					It("should succeed if it still has the relevant patch", func() {
						Expect(fsys.Remove(filepath.Join(tmpDir, "cache", patch.PluginsDir, srvManifestV2.Items[0].Digest))).To(Succeed())
						_, err := srv.CalculateUpdate(context.Background(), srvManifestV1)
						Expect(err).NotTo(HaveOccurred())
					})
					It("should return an error if it does not have the relevant patch", func() {
						_, err := srv.CalculateUpdate(context.Background(), &controlv1.UpdateManifest{
							Items: []*controlv1.UpdateManifestEntry{
								{
									Package: test1Package,
									Digest:  "deadbeef",
									Path:    "plugin_test1",
								},
							},
						})
						Expect(status.Code(err)).To(Equal(codes.Internal))
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("lost plugin in cache, cannot generate patch: %s", test1Package)))
					})

					It("should return an internal error when issuing create operations", func() {
						_, err := srv.CalculateUpdate(context.Background(), &controlv1.UpdateManifest{
							Items: []*controlv1.UpdateManifestEntry{},
						})
						Expect(status.Code(err)).To(Equal(codes.Internal))
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("lost plugin in cache: %s", test1Package)))

					})
				})
				When("the server is unable to read a patch on disk", func() {
					It("should return an internal error when fetching patches", func() {
						path := filepath.Join(tmpDir, "cache", patch.PatchesDir, fmt.Sprintf("%s-to-%s", srvManifestV1.Items[0].Digest, srvManifestV2.Items[0].Digest))
						Expect(fsys.Chmod(path, 0)).To(Succeed())
						DeferCleanup(func() {
							Expect(fsys.Chmod(path, 0644)).To(Succeed())
						})
						_, err := srv.CalculateUpdate(context.Background(), srvManifestV1)
						Expect(status.Code(err)).To(Equal(codes.Internal))
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("internal error in plugin cache, cannot sync: %s", test1Package)))
					})
				})
			})
			When("restarting the server", func() {
				It("should repopulate missing plugins", func() {
					var err error
					srv, err = newServer()
					Expect(err).NotTo(HaveOccurred())

					// plugin manifest is lazy-initialized, call CalculateExpectedManifest to
					// trigger initialization
					// TODO: if that logic is modified, update this test accordingly
					manifest, err := srv.CalculateExpectedManifest(context.Background(), urn.Plugin)
					Expect(err).NotTo(HaveOccurred())
					Expect(manifest).To(Equal(srvManifestV2))

					// the server should have repopulated the missing plugin
					plugins, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PluginsDir))
					Expect(err).NotTo(HaveOccurred())
					Expect(plugins).To(HaveLen(4))
					names := []string{plugins[0].Name(), plugins[1].Name(), plugins[2].Name(), plugins[3].Name()}
					Expect(names).To(ContainElements(srvManifestV1.Items[0].Digest, srvManifestV1.Items[1].Digest, srvManifestV2.Items[0].Digest, srvManifestV2.Items[1].Digest))
				})
			})
			When("multiple clients sync the same patches at the same time", func() {
				It("should compute the patch only once and send it to all clients", func() {
					start := make(chan struct{})

					fsys.Remove(filepath.Join(tmpDir, "cache", patch.PatchesDir, fmt.Sprintf("%s-to-%s", srvManifestV1.Items[0].Digest, srvManifestV2.Items[0].Digest)))
					fsys.Remove(filepath.Join(tmpDir, "cache", patch.PatchesDir, fmt.Sprintf("%s-to-%s", srvManifestV1.Items[1].Digest, srvManifestV2.Items[1].Digest)))

					exp := gmeasure.NewExperiment("inflight sync request deduplication")
					var wg sync.WaitGroup
					wg.Add(100)
					for i := 0; i < 100; i++ {
						go func() {
							defer wg.Done()
							<-start
							startTime := time.Now()
							patches, err := srv.CalculateUpdate(context.Background(), srvManifestV1)
							exp.RecordDuration("SyncPluginManifest", time.Since(startTime), gmeasure.Precision(time.Nanosecond))
							Expect(err).NotTo(HaveOccurred())
							Expect(patches.Items).To(HaveLen(2))
						}()
					}

					runtime.Gosched()
					close(start)
					wg.Wait()

					// all durations should be nearly exactly the same
					// AddReportEntry(exp.Name, exp)
					stats := exp.Get("SyncPluginManifest").Stats().DurationBundle
					// calculate the coefficient of variation
					cv := stats[gmeasure.StatStdDev] / stats[gmeasure.StatMean]
					Expect(cv).To(BeNumerically("<", 0.1), "request durations should be nearly identical")
				})
			})
			Specify("the stream interceptor should reject requests without matching plugin manifests", func() {
				By("creating a new grpc server")
				lis := bufconn.Listen(1024 * 1024)
				s := grpc.NewServer(
					grpc.ChainStreamInterceptor(
						func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
							sc := &streams.ServerStreamWithContext{
								Ctx:    context.WithValue(ss.Context(), cluster.ClusterIDKey, "cluster-1"),
								Stream: ss,
							}
							return handler(srv, sc)
						},
						updateSrv.StreamServerInterceptor(),
					),
					grpc.Creds(insecure.NewCredentials()),
				)

				testgrpc.RegisterStreamServiceServer(s, &testgrpc.StreamServer{
					ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
						md, ok := update.ManifestMetadataFromContext(stream.Context())
						Expect(ok).To(BeTrue())
						return stream.Send(&testgrpc.StreamResponse{
							Response: md.Digest(),
						})
					},
				})
				go s.Serve(lis)
				defer lis.Close()

				conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return lis.DialContext(ctx)
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
				Expect(err).NotTo(HaveOccurred())
				defer conn.Close()

				client := testgrpc.NewStreamServiceClient(conn)

				By("sending a request with a missing manifest digest")
				{
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
						controlv1.UpdateStrategyKeyForType(urn.Agent), "noop",
						controlv1.UpdateStrategyKeyForType(urn.Plugin), srv.Strategy(),
						controlv1.ManifestDigestKeyForType(urn.Agent), agentManifest.Digest(),
					))
					stream, err := client.Stream(ctx, grpc.WaitForReady(true))
					Expect(err).NotTo(HaveOccurred())
					_, err = stream.Recv()
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
				}

				By("sending a request with an outdated manifest digest")
				{
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
						controlv1.UpdateStrategyKeyForType(urn.Agent), "noop",
						controlv1.UpdateStrategyKeyForType(urn.Plugin), srv.Strategy(),
						controlv1.ManifestDigestKeyForType(urn.Agent), agentManifest.Digest(),
						controlv1.ManifestDigestKeyForType(urn.Plugin), srvManifestV1.Digest(),
					))
					stream, err := client.Stream(ctx, grpc.WaitForReady(true))
					Expect(err).NotTo(HaveOccurred())
					_, err = stream.Recv()
					Expect(err).To(testutil.MatchStatusCode(codes.FailedPrecondition))
				}

				By("sending a request with a matching manifest digest")
				{
					ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
						controlv1.UpdateStrategyKeyForType(urn.Agent), "noop",
						controlv1.UpdateStrategyKeyForType(urn.Plugin), srv.Strategy(),
						controlv1.ManifestDigestKeyForType(urn.Agent), agentManifest.Digest(),
						controlv1.ManifestDigestKeyForType(urn.Plugin), srvManifestV2.Digest(),
					))
					stream, err := client.Stream(ctx, grpc.WaitForReady(true))
					Expect(err).NotTo(HaveOccurred())
					resp, err := stream.Recv()
					Expect(err).NotTo(HaveOccurred())
					digest := (&controlv1.UpdateManifest{
						Items: append(srvManifestV2.Items, agentManifest.Items...),
					}).Digest()
					Expect(resp.Response).To(Equal(digest))
				}
			})
			It("should garbage collect old plugins and patches", func() {
				store := mock_storage.NewTestClusterStore(ctrl)
				store.CreateCluster(context.Background(), &corev1.Cluster{
					Id: "cluster-1",
					Metadata: &corev1.ClusterMetadata{
						LastKnownConnectionDetails: &corev1.LastKnownConnectionDetails{
							PluginVersions: map[string]string{
								test1Package: v1Manifest.Items[0].Metadata.Digest,
								test2Package: v1Manifest.Items[1].Metadata.Digest,
							},
						},
					},
				})

				srv.RunGarbageCollection(context.Background(), store)

				// should keep everything
				{
					plugins, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PluginsDir))
					Expect(err).NotTo(HaveOccurred())
					Expect(plugins).To(HaveLen(4))
				}

				srv.RunGarbageCollection(context.Background(), mock_storage.NewTestClusterStore(ctrl))

				// all patches and old plugins should be removed, since the cluster store is empty
				plugins, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PluginsDir))
				Expect(err).NotTo(HaveOccurred())
				Expect(plugins).To(HaveLen(2))
				names := []string{plugins[0].Name(), plugins[1].Name()}
				Expect(names).To(ContainElements(srvManifestV2.Items[0].Digest, srvManifestV2.Items[1].Digest))

				patches, err := fsys.ReadDir(filepath.Join(tmpDir, "cache", patch.PatchesDir))
				Expect(err).NotTo(HaveOccurred())
				Expect(patches).To(HaveLen(0))
			})
		})
		Context("error handling", func() {
			When("creating a new server", func() {
				When("an unknown cache backend is specified", func() {
					It("should return an error", func() {
						_, err := server.NewFilesystemPluginSyncServer(context.Background(), &configv1.CacheSpec{
							Backend: configv1.CacheBackend(2).Enum(),
						}, patcher, testlog.Log)
						Expect(err).To(MatchError("unknown cache backend: unknown"))
					})
				})
				When("the filesystem cache cannot be created", func() {
					It("should return an error", func() {
						_, err := server.NewFilesystemPluginSyncServer(context.Background(), &configv1.CacheSpec{
							Backend: configv1.CacheBackend_Filesystem.Enum(),
							Filesystem: &configv1.FilesystemCacheSpec{
								Dir: lo.ToPtr("/dev/null"),
							},
						}, patcher, testlog.Log)
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})
	}
}

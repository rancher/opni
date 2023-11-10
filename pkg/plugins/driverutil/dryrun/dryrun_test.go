package dryrun_test

import (
	"context"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/dryrun"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/rancher/opni/pkg/util/protorand"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type extConfigServer struct {
	*driverutil.ContextKeyableConfigServer[
		*ext.SampleGetRequest,
		*ext.SampleSetRequest,
		*ext.SampleResetRequest,
		*ext.SampleHistoryRequest,
		*ext.SampleConfigurationHistoryResponse,
		*ext.SampleConfiguration,
	]
}

func (s extConfigServer) DryRun(ctx context.Context, req *ext.SampleDryRunRequest) (*ext.SampleDryRunResponse, error) {
	res, err := s.ServerDryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &ext.SampleDryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: res.ValidationErrors.ToProto(),
	}, nil
}

var _ = Describe("DryRunClient", Label("unit"), func() {
	rand := protorand.New[*ext.SampleConfiguration]()
	rand.ExcludeMask(&fieldmaskpb.FieldMask{
		Paths: []string{
			"revision",
			"enabled",
		},
	})
	rand.Seed(GinkgoRandomSeed())
	mustGen := func() *ext.SampleConfiguration {
		t := rand.MustGen()
		driverutil.UnsetRevision(t)
		return t
	}
	var setDefaults func(*ext.SampleConfiguration)
	var newDefaults func() *ext.SampleConfiguration
	{
		defaults := mustGen()
		setDefaults = func(t *ext.SampleConfiguration) {
			merge.MergeWithReplace(t, defaults)
		}
		newDefaults = func() *ext.SampleConfiguration {
			return util.ProtoClone(defaults)
		}
	}
	_ = newDefaults

	var extClient ext.ConfigClient

	BeforeEach(func() {
		var srv extConfigServer
		srv.ContextKeyableConfigServer = srv.ContextKeyableConfigServer.Build(newValueStore(), newKeyValueStore(), setDefaults)
		grpcSrv := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
		ext.RegisterConfigServer(grpcSrv, srv)

		listener := bufconn.Listen(1024)
		go grpcSrv.Serve(listener)

		conn, err := grpc.DialContext(context.Background(), "bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())

		extClient = ext.NewConfigClient(conn)
		defaults := newDefaults()

		_, err = extClient.SetDefaultConfiguration(context.Background(), &ext.SampleSetRequest{
			Spec: defaults,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	specs := func(useShim bool) {
		When("setting the default config using the dry run client", func() {
			It("should return a diff and not commit the changes", func(ctx SpecContext) {
				dryRunClient := dryrun.NewDryRunClient(extClient)

				By("checking the current default config")
				currentDefault, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})

				By("issuing a dry-run request to set the default config")
				proposedUpdates := &ext.SampleConfiguration{
					Revision:    currentDefault.Revision,
					StringField: lo.ToPtr("proposed"),
					MapField: map[string]string{
						"proposed-key": "proposed-value",
					},
					RepeatedField: []string{"proposed-item1", "proposed-item2"},
					MessageField: &ext.SampleMessage{
						Field3: &ext.Sample3FieldMsg{
							Field2: 12345,
						},
					},
				}
				if useShim {
					shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
					_, err = shimClient.SetDefaultConfiguration(ctx, &ext.SampleSetRequest{
						Spec: proposedUpdates,
					})
				} else {
					_, err = dryRunClient.SetDefaultConfiguration(ctx, &ext.SampleSetRequest{
						Spec: proposedUpdates,
					})
				}
				Expect(err).NotTo(HaveOccurred())
				response := dryRunClient.Response()

				Expect(response.Current).To(testutil.ProtoEqual(currentDefault))

				By("checking that the proposed modifications are correct")
				Expect(response.Modified).To(testutil.ProtoEqual(proposedUpdates))

				By("checking that the default config has not been modified")
				currentDefault2, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(currentDefault2).To(testutil.ProtoEqual(currentDefault))
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})
			})
		})

		When("setting the active config using the dry run client", func() {
			It("should return a diff and not commit the changes", func(ctx SpecContext) {
				dryRunClient := dryrun.NewDryRunClient(extClient)

				By("checking the current default config")
				currentDefault, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})

				By("issuing a dry-run request to set the active config")
				proposedUpdates := &ext.SampleConfiguration{
					StringField: lo.ToPtr("proposed"),
					MapField: map[string]string{
						"proposed-key": "proposed-value",
					},
					RepeatedField: []string{"proposed-item1", "proposed-item2"},
					MessageField: &ext.SampleMessage{
						Field3: &ext.Sample3FieldMsg{
							Field2: 12345,
						},
					},
				}
				if useShim {
					shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
					_, err = shimClient.SetConfiguration(ctx, &ext.SampleSetRequest{
						Key:  lo.ToPtr("key1"),
						Spec: proposedUpdates,
					})
				} else {
					_, err = dryRunClient.SetConfiguration(ctx, &ext.SampleSetRequest{
						Key:  lo.ToPtr("key1"),
						Spec: proposedUpdates,
					})
				}
				Expect(err).NotTo(HaveOccurred())
				response := dryRunClient.Response()

				// we haven't set any active config yet, so the current active config
				// should be the same as the default config (except revision)
				expectedCurrent := util.ProtoClone(currentDefault)
				driverutil.UnsetRevision(expectedCurrent)
				Expect(response.Current).To(testutil.ProtoEqual(expectedCurrent))

				By("checking that the proposed modifications are correct")
				expectedModified := util.ProtoClone(expectedCurrent)
				merge.MergeWithReplace(expectedModified, proposedUpdates)
				Expect(response.Modified).To(testutil.ProtoEqual(expectedModified))

				By("checking that the active config has not been modified")
				currentActive, err := extClient.GetConfiguration(ctx, &ext.SampleGetRequest{
					Key: lo.ToPtr("key1"),
				})
				Expect(*currentActive.Revision.Revision).To(BeEquivalentTo(0))
				driverutil.UnsetRevision(currentActive)
				Expect(err).NotTo(HaveOccurred())
				Expect(currentActive).To(testutil.ProtoEqual(expectedCurrent))
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})
			})
		})

		When("resetting the default config using the dry run client", func() {
			It("should return a diff and not commit the changes", func(ctx SpecContext) {
				dryRunClient := dryrun.NewDryRunClient(extClient)

				By("checking the current default config")
				currentDefault, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})

				By("issuing a dry-run request to reset the default config")
				if useShim {
					shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
					_, err = shimClient.ResetDefaultConfiguration(ctx, &emptypb.Empty{})
				} else {
					_, err = dryRunClient.ResetDefaultConfiguration(ctx, &emptypb.Empty{})
				}
				Expect(err).NotTo(HaveOccurred())
				response := dryRunClient.Response()

				// revisions are not included in dry-run results for reset operations
				expectedCurrent := util.ProtoClone(currentDefault)
				driverutil.UnsetRevision(expectedCurrent)
				Expect(response.Current).To(testutil.ProtoEqual(expectedCurrent))

				By("checking that the proposed modifications are empty")
				expectedModified := newDefaults()
				expectedModified.RedactSecrets()
				Expect(response.Modified).To(testutil.ProtoEqual(expectedModified))

				By("checking that the default config has not been modified")
				currentDefault2, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				Expect(currentDefault2).To(testutil.ProtoEqual(currentDefault))
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})
			})
		})

		When("resetting the active config using the dry run client", func() {
			It("should return a diff and not commit the changes", func(ctx SpecContext) {
				dryRunClient := dryrun.NewDryRunClient(extClient)

				By("setting the active config")
				active := mustGen()
				_, err := extClient.SetConfiguration(ctx, &ext.SampleSetRequest{
					Key:  lo.ToPtr("key1"),
					Spec: active,
				})
				Expect(err).NotTo(HaveOccurred())

				By("checking the current default config")
				currentDefault, err := extClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
				Expect(err).NotTo(HaveOccurred())
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentDefault *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentDefault, err = shimClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					} else {
						dryRunCurrentDefault, err = dryRunClient.GetDefaultConfiguration(ctx, &ext.SampleGetRequest{})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentDefault).To(testutil.ProtoEqual(currentDefault))
				})

				By("checking the current active config")
				currentActive, err := extClient.GetConfiguration(ctx, &ext.SampleGetRequest{
					Key: lo.ToPtr("key1"),
				})
				Expect(err).NotTo(HaveOccurred())
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentActive *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentActive, err = shimClient.GetConfiguration(ctx, &ext.SampleGetRequest{
							Key: lo.ToPtr("key1"),
						})
					} else {
						dryRunCurrentActive, err = dryRunClient.GetConfiguration(ctx, &ext.SampleGetRequest{
							Key: lo.ToPtr("key1"),
						})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentActive).To(testutil.ProtoEqual(currentActive))
				})

				By("issuing a dry-run request to reset the active config")
				if useShim {
					shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
					_, err = shimClient.ResetConfiguration(ctx, &ext.SampleResetRequest{
						Key: lo.ToPtr("key1"),
					})
				} else {
					_, err = dryRunClient.ResetConfiguration(ctx, &ext.SampleResetRequest{
						Key: lo.ToPtr("key1"),
					})

				}
				Expect(err).NotTo(HaveOccurred())
				response := dryRunClient.Response()

				By("checking that the proposed modifications are correct")
				expectedCurrent := util.ProtoClone(currentActive)
				driverutil.UnsetRevision(expectedCurrent)
				Expect(response.Current).To(testutil.ProtoEqual(expectedCurrent))

				expectedModified := util.ProtoClone(currentDefault)
				driverutil.UnsetRevision(expectedModified)
				Expect(response.Modified).To(testutil.ProtoEqual(expectedModified))

				By("checking that the active config has not been modified")
				currentActive2, err := extClient.GetConfiguration(ctx, &ext.SampleGetRequest{
					Key: lo.ToPtr("key1"),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(currentActive2).To(testutil.ProtoEqual(currentActive))
				By("checking that the dry-run client does not modify Get requests", func() {
					var dryRunCurrentActive *ext.SampleConfiguration
					if useShim {
						shimClient := ext.NewConfigClient(dryrun.NewDryRunClientShim(dryRunClient, ext.ConfigContextInjector))
						dryRunCurrentActive, err = shimClient.GetConfiguration(ctx, &ext.SampleGetRequest{
							Key: lo.ToPtr("key1"),
						})
					} else {
						dryRunCurrentActive, err = dryRunClient.GetConfiguration(ctx, &ext.SampleGetRequest{
							Key: lo.ToPtr("key1"),
						})
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(dryRunCurrentActive).To(testutil.ProtoEqual(currentActive))
				})
			})
		})
	}

	Context("without using the client shim", func() {
		specs(false)
	})

	Context("using the client shim", func() {
		specs(true)
	})
})

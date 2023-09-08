package driverutil_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/rancher/opni/pkg/util/protorand"
)

var _ = Describe("Base Config Server", Ordered, func() {
	var server *driverutil.BaseConfigServer[
		*ext.SampleResetRequest,
		*ext.SampleConfigurationHistoryResponse,
		*ext.SampleConfiguration,
	]
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

	BeforeEach(func() {
		server = driverutil.NewBaseConfigServer[
			*ext.SampleResetRequest,
			*ext.SampleConfigurationHistoryResponse,
		](newValueStore(), newValueStore(), setDefaults)
	})
	Context("GetConfiguration", func() {
		It("should forward the request to the tracker", func(ctx context.Context) {
			By("generating a random configuration")
			config := rand.MustGen()
			By("setting the active configuration")
			Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(config))).To(Succeed())
			By("getting the configuration")
			res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			config.RedactSecrets()
			config.Revision = res.Revision

			Expect(res).To(testutil.ProtoEqual(config))
		})
	})

	Context("GetDefaultConfiguration", func() {
		It("should forward the request to the tracker", func(ctx context.Context) {
			By("getting the default configuration")
			defaults := newDefaults()
			res, err := server.GetDefaultConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			defaults.RedactSecrets()
			defaults.Revision = res.Revision
			Expect(res).To(testutil.ProtoEqual(defaults))
		})
	})
	Context("Install", func() {
		It("should set the enabled field of the config to true and apply it", func(ctx context.Context) {
			By("generating a random configuration")
			config := mustGen()
			By("setting the active configuration")
			Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(config))).To(Succeed())
			By("installing the configuration")
			_, err := server.Install(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			By("getting the configuration")
			res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			config.RedactSecrets()
			config.Revision = res.Revision
			By("checking the enabled field")
			config.Enabled = lo.ToPtr(true)
			Expect(res).To(testutil.ProtoEqual(config))
		})
	})
	Context("Uninstall", func() {
		It("should set the enabled field of the config to false and apply it", func(ctx context.Context) {
			By("generating a random configuration")
			config := mustGen()
			By("setting the active configuration")
			Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(config))).To(Succeed())
			By("uninstalling the configuration")
			_, err := server.Uninstall(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			By("getting the configuration")
			res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			config.RedactSecrets()
			config.Revision = res.Revision
			By("checking the enabled field")
			config.Enabled = lo.ToPtr(false)
			Expect(res).To(testutil.ProtoEqual(config))
		})
	})
	Context("ResetConfiguration", func() {
		When("no mask or patch is provided", func() {
			It("should forward the request to the tracker as-is", func(ctx context.Context) {
				By("modifying the active configuration")
				Expect(server.Tracker().ApplyConfig(ctx, mustGen())).To(Succeed())
				By("resetting the active configuration")
				_, err := server.ResetConfiguration(ctx, &ext.SampleResetRequest{})
				Expect(err).NotTo(HaveOccurred())
				By("getting the configuration")
				res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				def := newDefaults()
				def.RedactSecrets()
				def.Revision = res.Revision
				Expect(res).To(testutil.ProtoEqual(def))

				By("checking that history is preserved")
				history, err := server.ConfigurationHistory(ctx, &driverutil.ConfigurationHistoryRequest{
					Target: driverutil.Target_ActiveConfiguration,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(history.Entries).To(HaveLen(2)) // apply, reset
			})
		})
		When("a mask is provided, and no patch is provided", func() {
			It("should ensure the enabled field is masked", func(ctx context.Context) {
				By("modifying the active configuration")
				active := mustGen()
				Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(active))).To(Succeed())
				By("installing")
				_, err := server.Install(ctx, &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())

				By("resetting the active configuration")
				_, err = server.ResetConfiguration(ctx, &ext.SampleResetRequest{
					Mask: &fieldmaskpb.FieldMask{
						Paths: []string{
							"messageField.field1",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				By("getting the configuration")
				res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				expected := newDefaults()
				expected.RedactSecrets()
				expected.Enabled = lo.ToPtr(true)
				expected.MessageField.Field1 = util.ProtoClone(active.MessageField.Field1)
				expected.Revision = res.Revision
				Expect(res).To(testutil.ProtoEqual(expected))

				By("checking that history is preserved")
				history, err := server.ConfigurationHistory(ctx, &driverutil.ConfigurationHistoryRequest{
					Target: driverutil.Target_ActiveConfiguration,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(history.Entries).To(HaveLen(3)) // apply, install, reset
			})
		})
		When("a mask and patch are provided", func() {
			It("should ensure the enabled field is masked, and the enabled field is cleared from the patch", func(ctx context.Context) {
				By("modifying the active configuration")
				active := mustGen()
				Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(active))).To(Succeed())
				By("installing")
				_, err := server.Install(ctx, &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())

				By("resetting the active configuration")
				_, err = server.ResetConfiguration(ctx, &ext.SampleResetRequest{
					Mask: &fieldmaskpb.FieldMask{
						Paths: []string{
							"messageField.field1",
							"messageField.field2.field1",
						},
					},
					Patch: &ext.SampleConfiguration{
						Enabled: lo.ToPtr(false),
						MessageField: &ext.SampleMessage{
							Field2: &ext.Sample2FieldMsg{
								Field1: 12345,
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				By("getting the configuration")
				res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
				Expect(err).NotTo(HaveOccurred())
				expected := newDefaults()
				expected.RedactSecrets()
				expected.Enabled = lo.ToPtr(true)
				expected.MessageField.Field1 = util.ProtoClone(active.MessageField.Field1)
				expected.MessageField.Field2.Field1 = 12345
				expected.Revision = res.Revision
				Expect(res).To(testutil.ProtoEqual(expected))

				By("checking that history is preserved")
				history, err := server.ConfigurationHistory(ctx, &driverutil.ConfigurationHistoryRequest{
					Target: driverutil.Target_ActiveConfiguration,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(history.Entries).To(HaveLen(3)) // apply, install, reset
			})
		})
	})
	Context("ResetDefaultConfiguration", func() {
		It("should forward the request to the tracker", func(ctx context.Context) {
			By("modifying the default configuration")
			defaults := newDefaults()
			Expect(server.Tracker().SetDefaultConfig(ctx, mustGen())).To(Succeed())
			By("resetting the default configuration")
			_, err := server.ResetDefaultConfiguration(ctx, &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			By("getting the default configuration")
			res, err := server.GetDefaultConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			defaults.RedactSecrets()
			defaults.Revision = res.Revision
			Expect(res).To(testutil.ProtoEqual(defaults))
		})
	})
	Context("SetConfiguration", func() {
		It("should unset the enabled field and forward the request to the tracker", MustPassRepeatedly(100), func(ctx context.Context) {
			By("generating a random configuration")
			config := mustGen()
			config.Enabled = nil
			By("setting the active configuration")
			Expect(server.Tracker().ApplyConfig(ctx, util.ProtoClone(config))).To(Succeed())
			By("setting the configuration")
			config.Enabled = lo.ToPtr(true)
			_, err := server.SetConfiguration(ctx, util.ProtoClone(config))
			Expect(err).NotTo(HaveOccurred())
			By("getting the configuration")
			res, err := server.GetConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			By("checking the enabled field")
			Expect(res.Enabled).To(BeNil())
		})
	})
	Context("SetDefaultConfiguration", func() {
		It("should unset the enabled field and forward the request to the tracker", func(ctx context.Context) {
			defaults := newDefaults()
			defaults.Enabled = lo.ToPtr(true)
			By("setting the default configuration")
			_, err := server.SetDefaultConfiguration(ctx, defaults)
			Expect(err).NotTo(HaveOccurred())
			By("getting the default configuration")
			res, err := server.GetDefaultConfiguration(ctx, &driverutil.GetRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Enabled).To(BeNil())
		})
	})
	Context("ConfigurationHistory", func() {
		It("should forward the request to the tracker, and translate the response type", func(ctx context.Context) {
			By("applying multiple configurations")
			configs := make([]*ext.SampleConfiguration, 10)
			for i := 0; i < 10; i++ {
				configs[i] = mustGen()
				Expect(server.Tracker().ApplyConfig(ctx, configs[i])).To(Succeed())
			}
			By("getting the configuration history without values")
			res, err := server.ConfigurationHistory(ctx, &driverutil.ConfigurationHistoryRequest{
				IncludeValues: false,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.GetEntries()).To(HaveLen(10))
			for i := 0; i < 10; i++ {
				Expect(res.Entries[i].GetRevision()).NotTo(BeNil())
				clone := util.ProtoClone(res.Entries[i])
				clone.Revision = nil
				Expect(clone).To(testutil.ProtoEqual(&ext.SampleConfiguration{}))
			}

			By("getting the configuration history with values")
			res, err = server.ConfigurationHistory(ctx, &driverutil.ConfigurationHistoryRequest{
				IncludeValues: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.GetEntries()).To(HaveLen(10))
			for i := 0; i < 10; i++ {
				expected := util.ProtoClone(configs[i])
				expected.RedactSecrets()
				expected.Revision = res.GetEntries()[i].GetRevision()
				Expect(res.GetEntries()[i]).To(testutil.ProtoEqual(expected))
			}
		})
	})
})

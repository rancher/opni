package driverutil_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// SampleConfiguration and SampleSubConfiguration as described in the previous examples

var _ = Describe("DefaultingConfigTracker", Ordered, func() {
	var (
		ctx           context.Context
		configTracker *driverutil.DefaultingConfigTracker[*ext.SampleConfiguration]
		// defaultStore  storage.ValueStoreT[*ext.SampleConfiguration]
		// activeStore   storage.ValueStoreT[*ext.SampleConfiguration]
	)

	setDefaults := func(s *ext.SampleConfiguration) {
		s.StringField = lo.ToPtr("default")
		s.MapField = map[string]string{
			"default-key": "default-value",
		}
		s.RepeatedField = []string{"default", "default"}
		s.SecretField = lo.ToPtr("default-secret")
	}

	var updateC chan *ext.SampleConfiguration
	BeforeEach(func() {
		updateC = make(chan *ext.SampleConfiguration, 10)

		defaultStore := inmemory.NewProtoValueStore[*ext.SampleConfiguration]()
		activeStore := inmemory.NewProtoValueStore[*ext.SampleConfiguration](func(prev, value *ext.SampleConfiguration) {
			updateC <- value
		})
		ctx = context.TODO()

		configTracker = driverutil.NewDefaultingConfigTracker[*ext.SampleConfiguration](defaultStore, activeStore, setDefaults)
	})
	When("getting the default config", func() {
		It("should return a default config if it is in the store", func() {
			expected := &ext.SampleConfiguration{
				StringField:   lo.ToPtr("foo"),
				RepeatedField: []string{"bar", "baz"},
			}
			Expect(configTracker.SetDefaultConfig(ctx, expected)).To(Succeed())

			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(conf).To(testutil.ProtoEqual(expected.WithRevision(1)))
		})

		It("should return a new default config if it is not found in the store", func() {
			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())

			expected := &ext.SampleConfiguration{}
			setDefaults(expected)
			expected.RedactSecrets()

			Expect(conf).To(testutil.ProtoEqual(expected.WithRevision(0)))
		})
	})

	When("setting the default config", func() {
		Specify("subsequent calls to GetDefaultConfig should return the new default", func() {
			newDefault := &ext.SampleConfiguration{
				StringField:   lo.ToPtr("newDefault"),
				RepeatedField: []string{"bar", "baz"},
			}

			err := configTracker.SetDefaultConfig(ctx, newDefault)
			Expect(err).NotTo(HaveOccurred())

			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf).To(testutil.ProtoEqual(newDefault.WithRevision(1)))
		})
		When("applying configurations with secrets", func() {
			It("should correctly redact secrets", func() {
				newDefault := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("newDefault"),
					RepeatedField: []string{"bar", "baz"},
					SecretField:   lo.ToPtr("supersecret"),
				}
				err := configTracker.SetDefaultConfig(ctx, newDefault)
				Expect(err).NotTo(HaveOccurred())

				conf, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				newDefault.RedactSecrets()
				Expect(conf).To(testutil.ProtoEqual(newDefault.WithRevision(1)))
			})
		})
	})

	When("getting the active config", func() {
		When("there is an active config in the store", func() {
			Specify("GetConfig should return the active config", func() {
				active := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("active"),
					RepeatedField: []string{"bar", "baz"},
				}
				Expect(configTracker.ApplyConfig(ctx, active)).To(Succeed())

				defaults := &ext.SampleConfiguration{}
				setDefaults(defaults)
				merge.MergeWithReplace(defaults, active)

				Expect(<-updateC).To(testutil.ProtoEqual(defaults))

				conf, err := configTracker.GetConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				defaults.RedactSecrets()
				Expect(conf).To(testutil.ProtoEqual(defaults.WithRevision(1)))
			})
			Specify("GetConfigOrDefault should return the active config", func() {
				expected := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("active"),
					RepeatedField: []string{"bar", "baz"},
				}
				Expect(configTracker.ApplyConfig(ctx, expected)).To(Succeed())

				defaults := &ext.SampleConfiguration{}
				setDefaults(defaults)
				merge.MergeWithReplace(defaults, expected)

				Expect(<-updateC).To(testutil.ProtoEqual(defaults))

				conf, err := configTracker.GetConfigOrDefault(ctx)
				defaults.RedactSecrets()
				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(testutil.ProtoEqual(defaults.WithRevision(1)))
			})
		})
		When("there is no active config in the store", func() {
			Specify("GetConfig should return an error", func() {
				conf, err := configTracker.GetConfig(ctx)
				Expect(err).To(MatchError(storage.ErrNotFound))
				Expect(conf).To(BeNil())
			})
			Specify("GetConfigOrDefault should return a default config", func() {
				defaultConfig := &ext.SampleConfiguration{}
				setDefaults(defaultConfig)
				Expect(configTracker.SetDefaultConfig(ctx, defaultConfig)).To(Succeed())
				conf, err := configTracker.GetConfigOrDefault(ctx)

				Expect(err).NotTo(HaveOccurred())
				defaultConfig.RedactSecrets()
				Expect(conf).To(testutil.ProtoEqual(defaultConfig.WithRevision(0)))
			})
		})
		When("an error occurs looking up the active config", func() {
			It("should return the error", func() {
				_, err := configTracker.GetConfig(ctx)
				Expect(err).To(MatchError(storage.ErrNotFound))
			})
		})
	})

	When("applying the active config", func() {
		When("there is no existing active config in the store", func() {
			It("should merge the incoming config with the defaults", func() {
				newActive := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("newActive"),
					RepeatedField: []string{"new", "active"},
					MapField:      map[string]string{"a": "b", "c": "d"},
				}
				defaultConfig := &ext.SampleConfiguration{}
				setDefaults(defaultConfig)
				defaultConfig.SecretField = lo.ToPtr("new-secret")
				Expect(configTracker.SetDefaultConfig(ctx, defaultConfig)).To(Succeed())

				mergedConfig := defaultConfig
				mergedConfig.StringField = newActive.StringField
				mergedConfig.RepeatedField = newActive.RepeatedField
				mergedConfig.MapField = newActive.MapField

				err := configTracker.ApplyConfig(ctx, newActive)
				Expect(err).NotTo(HaveOccurred())
				Expect(<-updateC).To(testutil.ProtoEqual(mergedConfig.WithoutRevision()))

				mergedConfig.RedactSecrets()
				Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(mergedConfig.WithRevision(1)))
			})
			When("there is no default config in the store", func() {
				It("should merge the incoming config with new defaults", func() {
					newActive := &ext.SampleConfiguration{
						StringField:   lo.ToPtr("newActive"),
						RepeatedField: []string{"new", "active"},
						MapField:      map[string]string{"a": "b", "c": "d"},
					}

					err := configTracker.ApplyConfig(ctx, newActive)
					Expect(err).NotTo(HaveOccurred())

					newDefaults := &ext.SampleConfiguration{}
					setDefaults(newDefaults)
					merge.MergeWithReplace(newDefaults, newActive)

					Expect(<-updateC).To(testutil.ProtoEqual(newDefaults))

					newDefaults.RedactSecrets()
					Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(newDefaults.WithRevision(1)))
				})
			})
			When("applying with redacted placeholders", func() {
				It("should preserve the underlying secret value", func() {
					Expect(configTracker.SetDefaultConfig(ctx, &ext.SampleConfiguration{
						StringField: lo.ToPtr("default"),
						SecretField: lo.ToPtr("default-secret"),
					})).To(Succeed())
					Expect(configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{
						Revision:    v1.NewRevision(0),
						StringField: lo.ToPtr("oldActive"),
						SecretField: lo.ToPtr("***"),
					})).To(Succeed())
					Expect(<-updateC).To(testutil.ProtoEqual(&ext.SampleConfiguration{
						StringField: lo.ToPtr("oldActive"),
						SecretField: lo.ToPtr("default-secret"),
					}))

					err := configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{
						Revision:    v1.NewRevision(1),
						StringField: lo.ToPtr("newActive"),
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(<-updateC).To(testutil.ProtoEqual(&ext.SampleConfiguration{
						StringField: lo.ToPtr("newActive"),
						SecretField: lo.ToPtr("default-secret"),
					}))

					newerActive := &ext.SampleConfiguration{
						Revision:    v1.NewRevision(2),
						StringField: lo.ToPtr("newerActive"),
						SecretField: lo.ToPtr("***"),
					}
					Expect(configTracker.ApplyConfig(ctx, newerActive)).To(Succeed())
					Expect(<-updateC).To(testutil.ProtoEqual(&ext.SampleConfiguration{
						StringField: lo.ToPtr("newerActive"),
						SecretField: lo.ToPtr("default-secret"),
					}))

					Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(&ext.SampleConfiguration{
						Revision:    v1.NewRevision(3),
						StringField: lo.ToPtr("newerActive"),
						SecretField: lo.ToPtr("***"),
					}))
				})
			})
		})
		When("there is an existing active config in the store", func() {
			It("should merge with the existing active config", func() {
				defaults := &ext.SampleConfiguration{}
				setDefaults(defaults)

				existing := defaults
				existing.StringField = lo.ToPtr("active")
				existing.RepeatedField = []string{"bar", "baz"}

				newActive := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("newActive"),
					RepeatedField: []string{"new", "active"},
				}
				Expect(configTracker.ApplyConfig(ctx, existing)).To(Succeed())
				Expect(<-updateC).To(testutil.ProtoEqual(existing))
				mergedConfig := existing
				merge.MergeWithReplace(mergedConfig, newActive)

				err := configTracker.ApplyConfig(ctx, newActive.WithRevision(1))
				Expect(err).NotTo(HaveOccurred())

				Expect(<-updateC).To(testutil.ProtoEqual(mergedConfig.WithoutRevision()))
				mergedConfig.RedactSecrets()
				Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(mergedConfig.WithRevision(2)))
			})
		})
	})
	When("setting the active config", func() {
		It("should ignore any existing active config and merge with the default", func() {
			def := &ext.SampleConfiguration{}
			setDefaults(def)
			Expect(configTracker.SetDefaultConfig(ctx, def)).To(Succeed())

			defClone := util.ProtoClone(def)
			defClone.StringField = lo.ToPtr("newActive")

			err := configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{
				StringField: lo.ToPtr("newActive"),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(<-updateC).To(testutil.ProtoEqual(defClone))
			defClone.RedactSecrets()
			Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(defClone.WithRevision(1)))
		})
	})
	When("resetting the active config", func() {
		It("should delete the config from the underlying store", func() {
			Expect(configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{
				StringField: lo.ToPtr("active"),
			})).To(Succeed())
			newActive := &ext.SampleConfiguration{}
			setDefaults(newActive)
			newActive.StringField = lo.ToPtr("active")
			Expect(<-updateC).To(testutil.ProtoEqual(newActive))

			err := configTracker.ResetConfig(ctx, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(<-updateC).To(BeNil())

			_, err = configTracker.GetConfig(ctx)
			Expect(err).To(testutil.MatchStatusCode(storage.ErrNotFound))
		})
		When("an error occurs deleting the config", func() {
			It("should return the error", func() {
				err := configTracker.ResetConfig(ctx, nil)
				Expect(err).To(testutil.MatchStatusCode(storage.ErrNotFound))
				Expect(updateC).NotTo(Receive())
			})
		})
		When("providing a field mask", func() {
			It("should preserve the fields in the mask", func() {
				conf := &ext.SampleConfiguration{
					StringField:   lo.ToPtr("foo"),
					SecretField:   lo.ToPtr("bar"),
					RepeatedField: []string{"bar", "baz"},
					MapField:      map[string]string{"a": "b", "c": "d"},
				}
				Expect(configTracker.ApplyConfig(ctx, conf)).To(Succeed())
				Expect(<-updateC).To(testutil.ProtoEqual(conf))

				err := configTracker.ResetConfig(ctx, &fieldmaskpb.FieldMask{
					Paths: []string{"stringField"},
				})
				Expect(err).NotTo(HaveOccurred())

				expected := &ext.SampleConfiguration{}
				setDefaults(expected)
				expected.StringField = lo.ToPtr("foo")

				Expect(<-updateC).To(testutil.ProtoEqual(expected))
			})
		})
	})
	When("resetting the default config", func() {
		It("should delete the config from the underlying store", func() {
			Expect(configTracker.SetDefaultConfig(ctx, &ext.SampleConfiguration{
				StringField: lo.ToPtr("modifiedDefault"),
			})).To(Succeed())

			err := configTracker.ResetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(updateC).NotTo(Receive())

			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf.StringField).NotTo(Equal("default"))
		})
		When("an error occurs deleting the config", func() {
			It("should return the error", func() {
				err := configTracker.ResetDefaultConfig(ctx)
				Expect(err).To(MatchError(storage.ErrNotFound))
				Expect(updateC).NotTo(Receive())
			})
		})
	})
})

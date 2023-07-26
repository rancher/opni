package driverutil_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"go.uber.org/mock/gomock"
)

// SampleConfiguration and SampleSubConfiguration as described in the previous examples

var _ = Describe("DefaultingConfigTracker", func() {
	var (
		ctx              context.Context
		configTracker    *driverutil.DefaultingConfigTracker[*ext.SampleConfiguration]
		mockCtrl         *gomock.Controller
		mockDefaultStore *mock_storage.MockValueStoreT[*ext.SampleConfiguration]
		mockActiveStore  *mock_storage.MockValueStoreT[*ext.SampleConfiguration]
		testStoreError   = errors.New("test store error")
	)

	setDefaults := func(s *ext.SampleConfiguration) {
		s.StringField = "default"
		s.MapField = map[string]string{
			"default-key": "default-value",
		}
		s.RepeatedField = []string{"default", "default"}
		s.SecretField = "default-secret"
	}

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockDefaultStore = mock_storage.NewMockValueStoreT[*ext.SampleConfiguration](mockCtrl)
		mockActiveStore = mock_storage.NewMockValueStoreT[*ext.SampleConfiguration](mockCtrl)
		ctx = context.TODO()

		configTracker = driverutil.NewDefaultingConfigTracker[*ext.SampleConfiguration](mockDefaultStore, mockActiveStore, setDefaults)
	})
	When("getting the default config", func() {
		It("should return a default config if it is in the store", func() {
			expected := &ext.SampleConfiguration{
				StringField:   "foo",
				RepeatedField: []string{"bar", "baz"},
			}
			mockDefaultStore.EXPECT().Get(ctx).Return(expected, nil).Times(1)

			conf, err := configTracker.GetDefaultConfig(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(conf).To(testutil.ProtoEqual(expected))
		})

		It("should handle an error from the store", func() {
			mockDefaultStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

			_, err := configTracker.GetDefaultConfig(ctx)

			Expect(err).To(MatchError(testStoreError))
		})

		It("should return a new default config if it is not found in the store", func() {
			mockDefaultStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)

			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())

			expected := &ext.SampleConfiguration{}
			setDefaults(expected)
			expected.RedactSecrets()

			Expect(conf).To(testutil.ProtoEqual(expected))
		})
	})

	When("setting the default config", func() {
		Specify("subsequent calls to GetDefaultConfig should return the new default", func() {
			newDefault := &ext.SampleConfiguration{
				StringField:   "newDefault",
				RepeatedField: []string{"bar", "baz"},
			}
			mockDefaultStore.EXPECT().Get(ctx).Return(newDefault, nil).Times(2)
			mockDefaultStore.EXPECT().Put(ctx, newDefault).Return(nil).Times(1)

			err := configTracker.SetDefaultConfig(ctx, newDefault)
			Expect(err).NotTo(HaveOccurred())

			conf, err := configTracker.GetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(conf).To(testutil.ProtoEqual(newDefault))
		})
		When("the new config has redacted placeholders but the stored config is missing values", func() {
			It("should fail to unredact the config", func() {
				newDefault := &ext.SampleConfiguration{
					StringField:   "newDefault",
					RepeatedField: []string{"bar", "baz"},
					SecretField:   "***",
				}
				mockDefaultStore.EXPECT().Get(ctx).DoAndReturn(func(ctx context.Context) (*ext.SampleConfiguration, error) {
					return &ext.SampleConfiguration{
						StringField: "oldDefault",
						SecretField: "",
					}, nil
				}).Times(1)

				err := configTracker.SetDefaultConfig(ctx, newDefault)
				Expect(err).To(MatchError("cannot unredact: missing value for secret field: SecretField"))
			})
		})
		When("an error occurs looking up the default config", func() {
			It("should return the error", func() {
				newDefault := &ext.SampleConfiguration{
					StringField:   "newDefault",
					RepeatedField: []string{"new", "default"},
				}
				mockDefaultStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

				err := configTracker.SetDefaultConfig(ctx, newDefault)

				Expect(err).To(MatchError(testStoreError))
			})
		})
	})

	When("getting the active config", func() {
		When("there is an active config in the store", func() {
			Specify("GetConfig should return the active config", func() {
				active := &ext.SampleConfiguration{
					StringField:   "active",
					RepeatedField: []string{"bar", "baz"},
				}
				mockActiveStore.EXPECT().Get(ctx).Return(active, nil).Times(1)

				conf, err := configTracker.GetConfig(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(testutil.ProtoEqual(active))
			})
			Specify("GetConfigOrDefault should return the active config", func() {
				expected := &ext.SampleConfiguration{
					StringField:   "active",
					RepeatedField: []string{"bar", "baz"},
				}
				mockActiveStore.EXPECT().Get(ctx).Return(expected, nil).Times(1)

				conf, err := configTracker.GetConfigOrDefault(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(testutil.ProtoEqual(expected))
			})
		})
		When("there is no active config in the store", func() {
			Specify("GetConfig should return an error", func() {
				mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
				conf, err := configTracker.GetConfig(ctx)
				Expect(err).To(MatchError(storage.ErrNotFound))
				Expect(conf).To(BeNil())
			})
			Specify("GetConfigOrDefault should return a default config", func() {
				mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
				defaultConfig := &ext.SampleConfiguration{}
				setDefaults(defaultConfig)
				mockDefaultStore.EXPECT().Get(ctx).Return(defaultConfig, nil).Times(1)

				conf, err := configTracker.GetConfigOrDefault(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(testutil.ProtoEqual(defaultConfig))
			})
		})
		When("an error occurs looking up the active config", func() {
			It("should return the error", func() {
				mockActiveStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(2)

				_, err := configTracker.GetConfig(ctx)
				Expect(err).To(MatchError(testStoreError))

				_, err = configTracker.GetConfigOrDefault(ctx)
				Expect(err).To(MatchError(testStoreError))
			})
		})
		When("an error occurs looking up the default config", func() {
			It("should return the error", func() {
				mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
				mockDefaultStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

				_, err := configTracker.GetConfigOrDefault(ctx)

				Expect(err).To(MatchError(testStoreError))
			})
		})
	})

	When("applying the active config", func() {
		When("there is no existing active config in the store", func() {
			It("should merge the incoming config with the defaults", func() {
				newActive := &ext.SampleConfiguration{
					StringField:   "newActive",
					RepeatedField: []string{"new", "active"},
					MapField:      map[string]string{"a": "b", "c": "d"},
				}
				defaultConfig := &ext.SampleConfiguration{}
				setDefaults(defaultConfig)
				mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
				mockDefaultStore.EXPECT().Get(ctx).Return(defaultConfig, nil).Times(1)

				mergedConfig := util.ProtoClone(defaultConfig)
				mergedConfig.StringField = newActive.StringField
				mergedConfig.RepeatedField = newActive.RepeatedField
				mergedConfig.MapField = newActive.MapField

				mockActiveStore.EXPECT().Put(ctx, testutil.ProtoEqual(mergedConfig)).Return(nil).Times(1)

				err := configTracker.ApplyConfig(ctx, newActive)

				Expect(err).NotTo(HaveOccurred())
			})
			When("there is no default config in the store", func() {
				It("should merge the incoming config with new defaults", func() {
					newActive := &ext.SampleConfiguration{
						StringField:   "newActive",
						RepeatedField: []string{"new", "active"},
						MapField:      map[string]string{"a": "b", "c": "d"},
					}
					mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
					mockDefaultStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)

					mergedConfig := util.ProtoClone(newActive)
					setDefaults(mergedConfig)
					mergedConfig.StringField = newActive.StringField
					mergedConfig.RepeatedField = newActive.RepeatedField
					mergedConfig.MapField = newActive.MapField

					mockActiveStore.EXPECT().Put(ctx, testutil.ProtoEqual(mergedConfig)).Return(nil).Times(1)

					err := configTracker.ApplyConfig(ctx, newActive)

					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("an error occurs looking up the default config", func() {
				It("should return the error", func() {
					newActive := &ext.SampleConfiguration{
						StringField:   "newActive",
						RepeatedField: []string{"new", "active"},
					}
					mockActiveStore.EXPECT().Get(ctx).Return(nil, storage.ErrNotFound).Times(1)
					mockDefaultStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

					err := configTracker.ApplyConfig(ctx, newActive)

					Expect(err).To(MatchError(testStoreError))
				})
			})
			When("the new config has redacted placeholders but the stored config is missing values", func() {
				It("should fail to unredact the config", func() {
					mockActiveStore.EXPECT().Get(ctx).DoAndReturn(func(ctx context.Context) (*ext.SampleConfiguration, error) {
						return &ext.SampleConfiguration{
							StringField:   "oldActive",
							RepeatedField: []string{"foo", "bar", "baz"},
							MapField:      map[string]string{"a": "b"},
							SecretField:   "",
						}, nil
					}).Times(1)

					newActive := &ext.SampleConfiguration{
						StringField:   "newActive",
						RepeatedField: []string{"foo", "bar", "baz"},
						MapField:      map[string]string{"a": "b"},
						SecretField:   "***",
					}

					err := configTracker.ApplyConfig(ctx, newActive)
					Expect(err).To(MatchError("cannot unredact: missing value for secret field: SecretField"))
				})
			})
		})
		When("there is an existing active config in the store", func() {
			It("should merge with the existing active config", func() {
				existing := &ext.SampleConfiguration{
					StringField:   "active",
					RepeatedField: []string{"bar", "baz"},
				}
				newActive := &ext.SampleConfiguration{
					StringField:   "newActive",
					RepeatedField: []string{"new", "active"},
				}
				mockActiveStore.EXPECT().Get(ctx).Return(existing, nil).Times(1)
				mergedConfig := existing
				merge.MergeWithReplace(mergedConfig, newActive)
				mockActiveStore.EXPECT().Put(ctx, mergedConfig).Return(nil).Times(1)

				err := configTracker.ApplyConfig(ctx, newActive)

				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("an error occurs looking up the active config", func() {
			It("should return the error", func() {
				newActive := &ext.SampleConfiguration{
					StringField:   "newActive",
					RepeatedField: []string{"new", "active"},
				}
				mockActiveStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

				err := configTracker.ApplyConfig(ctx, newActive)

				Expect(err).To(MatchError(testStoreError))
			})
		})
	})
	When("setting the active config", func() {
		It("should ignore any existing active config and merge with the default", func() {
			def := &ext.SampleConfiguration{}
			setDefaults(def)
			mockDefaultStore.EXPECT().Get(ctx).Return(def, nil).Times(1)

			defClone := util.ProtoClone(def)
			defClone.StringField = "newActive"
			mockActiveStore.EXPECT().Put(ctx, testutil.ProtoEqual(defClone)).Return(nil).Times(1)

			err := configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{
				StringField: "newActive",
			})
			Expect(err).NotTo(HaveOccurred())
		})
		When("an error occurs looking up the default config", func() {
			It("should return the error", func() {
				mockDefaultStore.EXPECT().Get(ctx).Return(nil, testStoreError).Times(1)

				err := configTracker.ApplyConfig(ctx, &ext.SampleConfiguration{})
				Expect(err).To(MatchError(testStoreError))
			})
		})
	})
	When("resetting the active config", func() {
		It("should delete the config from the underlying store", func() {
			mockActiveStore.EXPECT().Delete(ctx).Return(nil).Times(1)

			err := configTracker.ResetConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
		When("an error occurs deleting the config", func() {
			It("should return the error", func() {
				mockActiveStore.EXPECT().Delete(ctx).Return(testStoreError).Times(1)

				err := configTracker.ResetConfig(ctx)
				Expect(err).To(MatchError(testStoreError))
			})
		})
	})
	When("resetting the default config", func() {
		It("should delete the config from the underlying store", func() {
			mockDefaultStore.EXPECT().Delete(ctx).Return(nil).Times(1)

			err := configTracker.ResetDefaultConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
		When("an error occurs deleting the config", func() {
			It("should return the error", func() {
				mockDefaultStore.EXPECT().Delete(ctx).Return(testStoreError).Times(1)

				err := configTracker.ResetDefaultConfig(ctx)
				Expect(err).To(MatchError(testStoreError))
			})
		})
	})
})

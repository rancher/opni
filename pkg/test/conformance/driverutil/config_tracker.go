package conformance_driverutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/rancher/opni/pkg/util/protorand"
	"github.com/samber/lo"
)

type newValueStore[T any] interface {
	func(cloneFunc func(T) T) storage.ValueStoreT[T]
}

type SuiteConfig[
	T driverutil.ConfigType[T],
	D newValueStore[D],
	A newValueStore[A],
] struct {
	NewDefaultStore D
	NewActiveStore  A
}

func DefaultingConfigTrackerTestSuite[
	T driverutil.ConfigType[T],
	D newValueStore[D],
	A newValueStore[A],
](config SuiteConfig[T, D, A]) func() {
	return func() {
		var transform = func(v storage.WatchEvent[storage.KeyRevision[T]]) T {
			if lo.IsEmpty(v.Current) {
				return lo.Empty[T]()
			}
			return v.Current.Value()
		}
		var withRevision = func(t T, rev int64) T {
			driverutil.SetRevision(t, rev)
			return t
		}
		var withoutRevision = func(t T) T {
			driverutil.UnsetRevision(t)
			return t
		}

		var (
			ctx           context.Context
			ca            context.CancelFunc
			configTracker *driverutil.DefaultingConfigTracker[T]
		)

		var mustGen func() T
		rand := protorand.New[T]()
		rand.Seed(GinkgoRandomSeed())
		mustGen = func() T {
			t := rand.MustGen()
			driverutil.UnsetRevision(t)
			return t
		}
		var setDefaults func(T)
		var newDefaults, newDefaultsRedacted func() T
		{
			defaults := mustGen()
			defaultsRedacted := util.ProtoClone(defaults)
			defaultsRedacted.RedactSecrets()
			setDefaults = func(t T) {
				merge.MergeWithReplace(t, defaults)
			}
			newDefaults = func() T {
				return util.ProtoClone(defaults)
			}
			newDefaultsRedacted = func() T {
				return util.ProtoClone(defaultsRedacted)
			}
		}

		var updateC <-chan storage.WatchEvent[storage.KeyRevision[T]]
		BeforeEach(func() {
			ctx, ca = context.WithCancel(context.Background())
			DeferCleanup(ca)
			defaultStore := inmemory.NewValueStore[T](util.ProtoClone)
			activeStore := inmemory.NewValueStore[T](util.ProtoClone)
			var err error
			updateC, err = activeStore.Watch(ctx)
			Expect(err).NotTo(HaveOccurred())
			configTracker = driverutil.NewDefaultingConfigTracker(defaultStore, activeStore, setDefaults)
		})
		When("getting the default config", func() {
			It("should return a default config if it is in the store", func() {
				expected := mustGen()
				Expect(configTracker.SetDefaultConfig(ctx, expected)).To(Succeed())

				conf, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())

				expected.RedactSecrets()
				driverutil.SetRevision(expected, 1)
				Expect(conf).To(testutil.ProtoEqual(expected))
			})

			It("should return a new default config if it is not found in the store", func() {
				conf, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(conf).To(testutil.ProtoEqual(newDefaultsRedacted()))
			})
		})

		When("setting the default config", func() {
			Specify("subsequent calls to GetDefaultConfig should return the new default", func() {
				newDefault := mustGen()

				err := configTracker.SetDefaultConfig(ctx, newDefault)
				Expect(err).NotTo(HaveOccurred())

				conf, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(testutil.ProtoEqual(withRevision(newDefault, 1)))
			})
			When("applying configurations with secrets", func() {
				// if T.SecretsRedactor is driverutil.NoopSecretsRedactor[T], skip this test
				var t driverutil.SecretsRedactor[T] = newDefaults()
				if _, ok := t.(driverutil.NoopSecretsRedactor[T]); ok {
					Skip("T is NoopSecretsRedactor")
				}
				It("should correctly redact secrets", func() {
					newDefault := mustGen()
					err := configTracker.SetDefaultConfig(ctx, newDefault)
					Expect(err).NotTo(HaveOccurred())

					conf, err := configTracker.GetDefaultConfig(ctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(conf).NotTo(testutil.ProtoEqual(newDefault))

					newDefault.RedactSecrets()
					Expect(conf).To(testutil.ProtoEqual(withRevision(newDefault, 1)))
				})
			})
		})

		When("getting the active config", func() {
			When("there is an active config in the store", func() {
				Specify("GetConfig should return the active config", func() {
					active := mustGen()
					Expect(configTracker.ApplyConfig(ctx, active)).To(Succeed())

					defaults := newDefaults()
					merge.MergeWithReplace(defaults, active)

					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defaults))))

					conf, err := configTracker.GetConfig(ctx)
					Expect(err).NotTo(HaveOccurred())
					defaults.RedactSecrets()
					Expect(conf).To(testutil.ProtoEqual(withRevision(defaults, 1)))
				})
				Specify("GetConfigOrDefault should return the active config", func() {
					expected := mustGen()
					Expect(configTracker.ApplyConfig(ctx, expected)).To(Succeed())

					defaults := newDefaults()
					merge.MergeWithReplace(defaults, expected)

					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defaults))))

					conf, err := configTracker.GetConfigOrDefault(ctx)
					defaults.RedactSecrets()
					Expect(err).NotTo(HaveOccurred())
					Expect(conf).To(testutil.ProtoEqual(withRevision(defaults, 1)))
				})
			})
			When("there is no active config in the store", func() {
				Specify("GetConfig should return an error", func() {
					conf, err := configTracker.GetConfig(ctx)
					Expect(err).To(MatchError(storage.ErrNotFound))
					Expect(conf).To(BeNil())
				})
				Specify("GetConfigOrDefault should return a default config", func() {
					defaultConfig := newDefaults()
					Expect(configTracker.SetDefaultConfig(ctx, defaultConfig)).To(Succeed())
					conf, err := configTracker.GetConfigOrDefault(ctx)

					Expect(err).NotTo(HaveOccurred())
					defaultConfig.RedactSecrets()
					Expect(conf).To(testutil.ProtoEqual(withRevision(defaultConfig, 0)))
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
					newActive := rand.MustGenPartial(0.25)
					defaultConfig := newDefaults()
					Expect(configTracker.SetDefaultConfig(ctx, defaultConfig)).To(Succeed())

					mergedConfig := defaultConfig
					merge.MergeWithReplace(mergedConfig, newActive)

					err := configTracker.ApplyConfig(ctx, newActive)
					Expect(err).NotTo(HaveOccurred())
					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(withoutRevision(mergedConfig)))))

					mergedConfig.RedactSecrets()
					Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(withoutRevision(mergedConfig)))
				})
				When("there is no default config in the store", func() {
					It("should merge the incoming config with new defaults", func() {
						newActive := rand.MustGenPartial(0.25)

						err := configTracker.ApplyConfig(ctx, newActive)
						Expect(err).NotTo(HaveOccurred())

						newDefaults := newDefaults()
						merge.MergeWithReplace(newDefaults, newActive)

						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(newDefaults))))

						newDefaults.RedactSecrets()
						Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(withRevision(newDefaults, 1)))
					})
				})
				When("applying with redacted placeholders", func() {
					// if T.SecretsRedactor is driverutil.NoopSecretsRedactor[T], skip this test
					var t driverutil.SecretsRedactor[T] = newDefaults()
					if _, ok := t.(driverutil.NoopSecretsRedactor[T]); ok {
						Skip("T is NoopSecretsRedactor")
					}
					It("should preserve the underlying secret value", func() {
						defaults := newDefaults()
						Expect(configTracker.SetDefaultConfig(ctx, defaults)).To(Succeed())

						newActive := withRevision(mustGen(), 0)
						// redact secrets before applying, which sets them to *** preserving the underlying value
						newActive.RedactSecrets()
						Expect(configTracker.ApplyConfig(ctx, newActive)).To(Succeed())
						var event storage.WatchEvent[storage.KeyRevision[T]]
						Eventually(updateC).Should(Receive(&event))

						// redact the defaults, then unredact them using the active config.
						// if the underlying secret was preserved, this should correctly
						// restore the secret fields in the original defaults.
						clonedDefaults := util.ProtoClone(defaults)
						clonedDefaults.RedactSecrets()
						clonedDefaults.UnredactSecrets(newActive)
						Expect(defaults).To(testutil.ProtoEqual(clonedDefaults))
					})
				})
			})
			When("there is an existing active config in the store", func() {
				It("should merge with the existing active config", func() {
					oldActive := mustGen()

					newActive := rand.MustGenPartial(0.5)
					Expect(configTracker.ApplyConfig(ctx, oldActive)).To(Succeed())
					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(oldActive))))
					mergedConfig := oldActive
					merge.MergeWithReplace(mergedConfig, newActive)

					err := configTracker.ApplyConfig(ctx, withRevision(newActive, 1))
					Expect(err).NotTo(HaveOccurred())

					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(withoutRevision(mergedConfig)))))
					mergedConfig.RedactSecrets()
					Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(withRevision(mergedConfig, 2)))
				})
			})
		})
		When("setting the active config", func() {
			It("should ignore any existing active config and merge with the default", func() {
				def := newDefaults()
				Expect(configTracker.SetDefaultConfig(ctx, def)).To(Succeed())

				defClone := util.ProtoClone(def)

				updates := rand.MustGenPartial(0.1)

				merge.MergeWithReplace(defClone, updates)

				err := configTracker.ApplyConfig(ctx, updates)
				Expect(err).NotTo(HaveOccurred())
				Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defClone))))
				defClone.RedactSecrets()
				Expect(configTracker.GetConfig(ctx)).To(testutil.ProtoEqual(withRevision(defClone, 1)))
			})
		})
		When("resetting the active config", func() {
			It("should delete the config from the underlying store", func() {
				updates := rand.MustGenPartial(0.1)

				Expect(configTracker.ApplyConfig(ctx, updates)).To(Succeed())
				newActive := newDefaults()
				merge.MergeWithReplace(newActive, updates)
				Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(newActive))))

				err := configTracker.ResetConfig(ctx, nil)
				Expect(err).NotTo(HaveOccurred())

				Eventually(updateC).Should(Receive(WithTransform(transform, BeNil())))

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
					conf := rand.MustGen()
					Expect(configTracker.ApplyConfig(ctx, conf)).To(Succeed())
					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(conf))))

					// generate a random mask
					updates := rand.MustGenPartial(0.1)
					mask := util.NewFieldMaskByPresence(updates.ProtoReflect())

					err := configTracker.ResetConfig(ctx, mask)
					Expect(err).NotTo(HaveOccurred())

					expected := newDefaults()
					merge.MergeWithReplace(expected, updates)

					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(expected))))
				})
			})
		})
		When("resetting the default config", func() {
			It("should delete the config from the underlying store", func() {
				originalDefault, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())

				modifiedDefault := rand.MustGenPartial(0.5)
				Expect(configTracker.SetDefaultConfig(ctx, modifiedDefault)).To(Succeed())

				err = configTracker.ResetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(updateC).NotTo(Receive())

				conf, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(conf).To(Equal(originalDefault))
			})
			When("an error occurs deleting the config", func() {
				It("should return the error", func() {
					err := configTracker.ResetDefaultConfig(ctx)
					Expect(err).To(MatchError(storage.ErrNotFound))
					Expect(updateC).NotTo(Receive())
				})
			})
		})
	}
}

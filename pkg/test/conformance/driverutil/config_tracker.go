package conformance_driverutil

import (
	"context"
	"fmt"
	"io"
	mathrand "math/rand"
	"sort"
	"sync"
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"github.com/rancher/opni/pkg/util/merge"
	"github.com/rancher/opni/pkg/util/protorand"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func DefaultingConfigTrackerTestSuite[
	T driverutil.ConfigType[T],
](newDefaultStore, newActiveStore func() storage.ValueStoreT[T]) func() {
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
		var redacted = func(t T) T {
			t.RedactSecrets()
			return t
		}
		var withoutRevision = func(t T) T {
			driverutil.UnsetRevision(t)
			return t
		}
		// var withoutTimestamp = func(t T) T {
		// 	driverutil.SetRevision(t, t.GetRevision().GetRevision())
		// 	return t
		// }

		var (
			wctx          context.Context
			wca           context.CancelFunc
			configTracker *driverutil.DefaultingConfigTracker[T]
		)

		rand := protorand.New[T]()
		rand.ExcludeMask(&fieldmaskpb.FieldMask{
			Paths: []string{
				"revision",
			},
		})
		rand.Seed(GinkgoRandomSeed())
		mustGen := func() T {
			t := rand.MustGen()
			driverutil.UnsetRevision(t)
			return t
		}
		mustGenPartial := func(p float64) T {
			t := rand.MustGenPartial(p)
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

		Context("Common Actions", func() {
			var defaultStore, activeStore storage.ValueStoreT[T]
			var updateC <-chan storage.WatchEvent[storage.KeyRevision[T]]
			BeforeEach(func() {
				wctx, wca = context.WithCancel(context.Background())
				DeferCleanup(wca)
				defaultStore = newDefaultStore()
				activeStore = newActiveStore()
				var err error
				updateC, err = activeStore.Watch(wctx)
				Expect(err).NotTo(HaveOccurred())
				configTracker = driverutil.NewDefaultingConfigTracker(defaultStore, activeStore, setDefaults)
			})
			When("getting the default config", func() {
				It("should return a default config if it is in the store", func() {
					expected := mustGen()
					Expect(configTracker.SetDefaultConfig(wctx, expected)).To(Succeed())

					conf, err := configTracker.GetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())

					expected.RedactSecrets()
					driverutil.CopyRevision(expected, conf)
					Expect(conf).To(testutil.ProtoEqual(expected))
				})

				It("should return a new default config if it is not found in the store", func() {
					conf, err := configTracker.GetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())

					Expect(conf).To(testutil.ProtoEqual(withRevision(newDefaultsRedacted(), 0)))
				})
			})

			When("setting the default config", func() {
				Specify("subsequent calls to GetDefaultConfig should return the new default", func() {
					newDefault := mustGen()

					err := configTracker.SetDefaultConfig(wctx, newDefault)
					Expect(err).NotTo(HaveOccurred())

					conf, err := configTracker.GetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())

					newDefault.RedactSecrets()
					driverutil.CopyRevision(newDefault, conf)
					Expect(conf).To(testutil.ProtoEqual(newDefault))
				})
				When("applying configurations with secrets", func() {
					It("should correctly redact secrets", func() {
						newDefault := mustGen()
						err := configTracker.SetDefaultConfig(wctx, newDefault)
						Expect(err).NotTo(HaveOccurred())

						conf, err := configTracker.GetDefaultConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(conf).NotTo(testutil.ProtoEqual(newDefault))

						newDefault.RedactSecrets()
						driverutil.CopyRevision(newDefault, conf)
						Expect(conf).To(testutil.ProtoEqual(newDefault))
					})
				})
			})

			When("getting the active config", func() {
				When("there is an active config in the store", func() {
					Specify("GetConfig should return the active config", func() {
						active := mustGen()
						Expect(configTracker.ApplyConfig(wctx, active)).To(Succeed())

						defaults := newDefaults()
						merge.MergeWithReplace(defaults, active)

						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defaults))))

						conf, err := configTracker.GetConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						defaults.RedactSecrets()
						driverutil.CopyRevision(defaults, conf)
						Expect(conf).To(testutil.ProtoEqual(defaults))
					})
					Specify("GetConfigOrDefault should return the active config", func() {
						expected := mustGen()
						Expect(configTracker.ApplyConfig(wctx, expected)).To(Succeed())

						defaults := newDefaults()
						merge.MergeWithReplace(defaults, expected)

						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defaults))))

						conf, err := configTracker.GetConfigOrDefault(wctx)
						Expect(err).NotTo(HaveOccurred())
						defaults.RedactSecrets()
						driverutil.CopyRevision(defaults, conf)
						Expect(conf).To(testutil.ProtoEqual(defaults))
					})
				})
				When("there is no active config in the store", func() {
					Specify("GetConfig should return an error", func() {
						conf, err := configTracker.GetConfig(wctx)
						Expect(storage.IsNotFound(err)).To(BeTrue())
						Expect(conf).To(BeNil())
					})
					Specify("GetConfigOrDefault should return a default config", func() {
						defaultConfig := newDefaults()
						Expect(configTracker.SetDefaultConfig(wctx, defaultConfig)).To(Succeed())
						conf, err := configTracker.GetConfigOrDefault(wctx)

						Expect(err).NotTo(HaveOccurred())
						defaultConfig.RedactSecrets()
						driverutil.CopyRevision(defaultConfig, conf)
						Expect(conf).To(testutil.ProtoEqual(defaultConfig))
					})
				})
				When("an error occurs looking up the active config", func() {
					It("should return the error", func() {
						_, err := configTracker.GetConfig(wctx)
						Expect(storage.IsNotFound(err)).To(BeTrue())
					})
				})
			})

			When("applying the active config", func() {
				When("there is no existing active config in the store", func() {
					It("should merge the incoming config with the defaults", func() {
						newActive := mustGenPartial(0.25)
						defaultConfig := newDefaults()
						Expect(configTracker.SetDefaultConfig(wctx, defaultConfig)).To(Succeed())

						mergedConfig := defaultConfig
						merge.MergeWithReplace(mergedConfig, newActive)

						err := configTracker.ApplyConfig(wctx, newActive)
						Expect(err).NotTo(HaveOccurred())
						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(withoutRevision(mergedConfig)))))

						activeConfig, err := configTracker.GetConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						mergedConfig.RedactSecrets()
						driverutil.CopyRevision(mergedConfig, activeConfig)

						Expect(activeConfig).To(testutil.ProtoEqual(mergedConfig))
					})
					When("there is no default config in the store", func() {
						It("should merge the incoming config with new defaults", func() {
							newActive := mustGenPartial(0.25)

							err := configTracker.ApplyConfig(wctx, newActive)
							Expect(err).NotTo(HaveOccurred())

							newDefaults := newDefaults()
							merge.MergeWithReplace(newDefaults, newActive)

							Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(newDefaults))))

							activeConfig, err := configTracker.GetConfig(wctx)
							Expect(err).NotTo(HaveOccurred())

							newDefaults.RedactSecrets()
							driverutil.CopyRevision(newDefaults, activeConfig)
							Expect(activeConfig).To(testutil.ProtoEqual(newDefaults))
						})
					})
					When("applying with redacted placeholders", func() {
						It("should preserve the underlying secret value", func() {
							defaults := newDefaults()
							Expect(configTracker.SetDefaultConfig(wctx, defaults)).To(Succeed())

							newActive := withRevision(mustGen(), 0)
							// redact secrets before applying, which sets them to *** preserving the underlying value
							newActive.RedactSecrets()
							Expect(configTracker.ApplyConfig(wctx, newActive)).To(Succeed())
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

						newActive := mustGenPartial(0.5)
						Expect(configTracker.ApplyConfig(wctx, oldActive)).To(Succeed())
						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(oldActive))))
						mergedConfig := oldActive
						merge.MergeWithReplace(mergedConfig, newActive)

						driverutil.CopyRevision(newActive, mergedConfig)
						err := configTracker.ApplyConfig(wctx, newActive)
						Expect(err).NotTo(HaveOccurred())

						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(withoutRevision(mergedConfig)))))

						activeConfig, err := configTracker.GetConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						mergedConfig.RedactSecrets()
						driverutil.CopyRevision(mergedConfig, activeConfig)

						Expect(activeConfig).To(testutil.ProtoEqual(mergedConfig))
					})
				})
			})
			When("setting the active config", func() {
				It("should ignore any existing active config and merge with the default", func() {
					def := newDefaults()
					Expect(configTracker.SetDefaultConfig(wctx, def)).To(Succeed())

					defClone := util.ProtoClone(def)

					updates := mustGenPartial(0.1)

					merge.MergeWithReplace(defClone, updates)

					err := configTracker.ApplyConfig(wctx, updates)
					Expect(err).NotTo(HaveOccurred())
					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(defClone))))

					activeConfig, err := configTracker.GetConfig(wctx)
					Expect(err).NotTo(HaveOccurred())
					defClone.RedactSecrets()
					driverutil.CopyRevision(defClone, activeConfig)

					Expect(activeConfig).To(testutil.ProtoEqual(defClone))
				})
			})
			When("resetting the active config", func() {
				It("should delete the config from the underlying store", func() {
					updates := mustGenPartial(0.1)

					Expect(configTracker.ApplyConfig(wctx, updates)).To(Succeed())
					newActive := newDefaults()
					merge.MergeWithReplace(newActive, updates)
					Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(newActive))))

					err := configTracker.ResetConfig(wctx, nil, lo.Empty[T]())
					Expect(err).NotTo(HaveOccurred())

					Eventually(updateC).Should(Receive(WithTransform(transform, BeNil())))

					_, err = configTracker.GetConfig(wctx)
					Expect(err).To(testutil.MatchStatusCode(storage.ErrNotFound))
				})
				When("an error occurs deleting the config", func() {
					It("should return the error", func() {
						err := configTracker.ResetConfig(wctx, nil, lo.Empty[T]())
						Expect(err).To(testutil.MatchStatusCode(storage.ErrNotFound))
						Expect(updateC).NotTo(Receive())
					})
				})
				When("providing a field mask", func() {
					It("should preserve the fields in the mask", func() {
						conf := mustGen()
						Expect(configTracker.ApplyConfig(wctx, conf)).To(Succeed())
						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(conf))))

						// generate a random mask
						mask := fieldmask.ByPresence(mustGenPartial(0.25).ProtoReflect())
						Expect(mask.IsValid(conf)).To(BeTrue(), mask.GetPaths())

						err := configTracker.ResetConfig(wctx, mask, lo.Empty[T]())
						Expect(err).NotTo(HaveOccurred())

						expected := newDefaults()
						fieldmask.ExclusiveKeep(conf, mask)
						merge.MergeWithReplace(expected, conf)

						Eventually(updateC).Should(Receive(WithTransform(transform, testutil.ProtoEqual(expected))))
					})
				})
			})
			When("resetting the default config", func() {
				It("should delete the config from the underlying store", func() {
					originalDefault, err := configTracker.GetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())

					modifiedDefault := mustGenPartial(0.5)
					Expect(configTracker.SetDefaultConfig(wctx, modifiedDefault)).To(Succeed())

					err = configTracker.ResetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(updateC).NotTo(Receive())

					conf, err := configTracker.GetDefaultConfig(wctx)
					Expect(err).NotTo(HaveOccurred())
					Expect(conf).To(Equal(originalDefault))
				})
				When("an error occurs deleting the config", func() {
					It("should return the error", func() {
						err := configTracker.ResetDefaultConfig(wctx)
						Expect(storage.IsNotFound(err)).To(BeTrue())
						Expect(updateC).NotTo(Receive())
					})
				})
			})
			When("using dry-run mode", func() {
				When("setting the default config", func() {
					It("should report changes without persisting them", func() {
						newDefault := mustGen()

						results, err := configTracker.DryRunSetDefaultConfig(wctx, newDefault)
						Expect(err).NotTo(HaveOccurred())
						Expect(results.Current).To(testutil.ProtoEqual(newDefaultsRedacted()))
						conf := results.Modified

						newDefault.RedactSecrets()
						driverutil.CopyRevision(newDefault, conf)
						Expect(conf).To(testutil.ProtoEqual(newDefault))

						conf, err = configTracker.GetDefaultConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(conf).To(testutil.ProtoEqual(withRevision(newDefaultsRedacted(), 0)))
					})
				})
				When("applying the active config", func() {
					It("should report changes without persisting them", func() {
						newActive := mustGen()

						results, err := configTracker.DryRunApplyConfig(wctx, newActive)
						Expect(err).NotTo(HaveOccurred())
						Expect(results.Current).To(testutil.ProtoEqual(withoutRevision(newDefaultsRedacted())))
						conf := results.Modified

						newActive.RedactSecrets()
						driverutil.CopyRevision(newActive, conf)
						Expect(conf).To(testutil.ProtoEqual(newActive))

						_, err = configTracker.GetConfig(wctx)
						Expect(err).To(testutil.MatchStatusCode(storage.ErrNotFound))
					})
				})
				When("resetting the default config", func() {
					It("should report changes without persisting them", func() {
						conf := mustGen()
						Expect(configTracker.SetDefaultConfig(wctx, conf)).To(Succeed())
						conf.RedactSecrets()

						results, err := configTracker.DryRunResetDefaultConfig(wctx)
						Expect(err).NotTo(HaveOccurred())

						Expect(results.Current).To(testutil.ProtoEqual(withoutRevision(conf)))
						Expect(results.Modified).To(testutil.ProtoEqual(withoutRevision(newDefaultsRedacted())))

						conf, err = configTracker.GetDefaultConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						Expect(conf).To(testutil.ProtoEqual(withoutRevision(conf)))
					})
				})
				When("resetting the active config", func() {
					When("neither mask nor patch are provided", func() {
						It("should report changes without persisting them", func() {
							conf := mustGen()
							Expect(configTracker.ApplyConfig(wctx, conf)).To(Succeed())
							conf.RedactSecrets()

							results, err := configTracker.DryRunResetConfig(wctx, nil, lo.Empty[T]())
							Expect(err).NotTo(HaveOccurred())

							Expect(results.Current).To(testutil.ProtoEqual(withoutRevision(conf)))
							Expect(results.Modified).To(testutil.ProtoEqual(withoutRevision(newDefaultsRedacted())))

							conf, err = configTracker.GetConfig(wctx)
							Expect(err).NotTo(HaveOccurred())
							Expect(conf).To(testutil.ProtoEqual(withoutRevision(conf)))
						})
					})
					When("a mask is provided, but no patch", func() {

					})
					When("both a mask and patch are provided", func() {

					})
				})
			})
			When("querying history", func() {
				When("values are requested", func() {
					It("should redact secrets", func() {
						cfg1 := mustGen()
						Expect(configTracker.SetDefaultConfig(wctx, cfg1)).To(Succeed())
						cfg1WithRev, err := configTracker.GetDefaultConfig(wctx)
						Expect(err).NotTo(HaveOccurred())
						cfg2 := mustGen()
						cfg2WithRev := util.ProtoClone(cfg2)
						driverutil.CopyRevision(cfg2WithRev, cfg1WithRev)
						Expect(configTracker.SetDefaultConfig(wctx, cfg2WithRev)).To(Succeed())
						Expect(configTracker.ApplyConfig(wctx, cfg1)).To(Succeed())
						Expect(configTracker.ApplyConfig(wctx, cfg2)).To(Succeed())

						historyDefault, err := configTracker.History(wctx, driverutil.Target_DefaultConfiguration, storage.IncludeValues(true))
						Expect(err).NotTo(HaveOccurred())
						historyActive, err := configTracker.History(wctx, driverutil.Target_ActiveConfiguration, storage.IncludeValues(true))
						Expect(err).NotTo(HaveOccurred())
						Expect(historyDefault).To(HaveLen(2))
						Expect(historyDefault[0].Value()).NotTo(testutil.ProtoEqual(cfg1))
						Expect(historyDefault[1].Value()).NotTo(testutil.ProtoEqual(cfg2))
						Expect(historyActive).To(HaveLen(2))
						Expect(historyActive[0].Value()).NotTo(testutil.ProtoEqual(cfg1))
						Expect(historyActive[1].Value()).NotTo(testutil.ProtoEqual(cfg2))

						cfg1.RedactSecrets()
						cfg2.RedactSecrets()

						Expect(historyDefault[0].Value()).To(testutil.ProtoEqual(cfg1))
						Expect(historyDefault[1].Value()).To(testutil.ProtoEqual(cfg2))
						Expect(historyActive[0].Value()).To(testutil.ProtoEqual(cfg1))
						Expect(historyActive[1].Value()).To(testutil.ProtoEqual(cfg2))
					})
				})
			})
		})

		Context("Watch Stress Testing", func() {
			var numDefaultWatchEvents, numActiveWatchEvents int
			var watchDefault, watchActive <-chan storage.WatchEvent[storage.KeyRevision[T]]

			var debugLogger io.Writer = io.Discard
			var _setDefault func(SpecContext, T)
			setDefault := func(ctx SpecContext) {
				_setDefault(ctx, mustGen())
			}
			_setDefault = func(ctx SpecContext, newDefault T) {
				fmt.Fprintln(debugLogger, "action: setDefault")
				defer func() { fmt.Fprintln(debugLogger, "action: setDefault (end)") }()

				currentDefaultRev := testutil.Must(configTracker.GetDefaultConfig(ctx)).GetRevision().GetRevision()
				Expect(configTracker.SetDefaultConfig(ctx, withRevision(newDefault, currentDefaultRev))).To(Succeed())
				newDefault.RedactSecrets()
				newDefaultRev := testutil.Must(configTracker.GetDefaultConfig(ctx)).GetRevision().GetRevision()
				select {
				case e := <-watchDefault:
					Expect(e.EventType).To(Equal(storage.WatchEventPut))
					Expect(withRevision(redacted(e.Current.Value()), e.Current.Revision())).To(testutil.ProtoEqual(withRevision(newDefault, newDefaultRev)))
				case <-ctx.Done():
					Fail(fmt.Sprintf("timed out waiting for default config: %v", ctx.Err()))
				}
				numDefaultWatchEvents++
			}
			resetDefault := func(ctx SpecContext) {
				fmt.Fprintln(debugLogger, "action: resetDefault")
				defer func() { fmt.Fprintln(debugLogger, "action: resetDefault (end)") }()
				lastDefault, err := configTracker.GetDefaultConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(configTracker.ResetDefaultConfig(ctx)).To(Succeed())
				select {
				case e := <-watchDefault:
					Expect(e.EventType).To(Equal(storage.WatchEventDelete))
					Expect(withRevision(redacted(e.Previous.Value()), e.Previous.Revision())).To(testutil.ProtoEqual(lastDefault))
				case <-ctx.Done():
					Fail(fmt.Sprintf("timed out waiting for default config: %v", ctx.Err()))
				}
				numDefaultWatchEvents++
			}
			var _setActive func(SpecContext, T)
			setActive := func(ctx SpecContext) {
				_setActive(ctx, mustGen())
			}
			_setActive = func(ctx SpecContext, newActive T) {
				fmt.Fprintln(debugLogger, "action: setActive")
				defer func() { fmt.Fprintln(debugLogger, "action: setActive (end)") }()
				for i := 0; ; i++ {
					currentActive, err := configTracker.GetConfig(ctx)
					if storage.IsNotFound(err) {
						err := configTracker.ApplyConfig(ctx, newActive)
						if err != nil {
							if storage.IsConflict(err) {
								continue
							}
							Fail(err.Error())
						}
					} else if err != nil {
						Fail(err.Error())
					} else {
						Expect(configTracker.ApplyConfig(ctx, withRevision(newActive, currentActive.GetRevision().GetRevision()))).To(Succeed())
					}
					break
				}
				actualActive, err := configTracker.GetConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
				select {
				case e := <-watchActive:
					Expect(e.EventType).To(Equal(storage.WatchEventPut))
					Expect(withRevision(redacted(e.Current.Value()), e.Current.Revision())).To(testutil.ProtoEqual(actualActive))
				case <-ctx.Done():
					Fail(fmt.Sprintf("timed out waiting for active config: %v", ctx.Err()))
				}
				numActiveWatchEvents++
			}
			var _resetActive func(SpecContext)
			resetActive := func(ctx SpecContext) {
				_resetActive(ctx)
			}
			_resetActive = func(ctx SpecContext) {
				fmt.Fprintln(debugLogger, "action: resetActive")
				defer func() { fmt.Fprintln(debugLogger, "action: resetActive (end)") }()

				lastActive, err := configTracker.GetConfig(ctx)
				Expect(err).NotTo(HaveOccurred())

				Expect(configTracker.ResetConfig(ctx, nil, lo.Empty[T]())).To(Succeed())

				select {
				case e := <-watchActive:
					Expect(e.EventType).To(Equal(storage.WatchEventDelete))
					Expect(e.Current).To(BeNil())
					Expect(withRevision(redacted(e.Previous.Value()), e.Previous.Revision())).To(testutil.ProtoEqual(lastActive))

				case <-ctx.Done():
					Fail(fmt.Sprintf("timed out waiting for active config: %v", ctx.Err()))
				}

				numActiveWatchEvents++
			}
			done := func(ctx SpecContext) {
				fmt.Fprintln(debugLogger, "action: done")
			}
			// state tracker for which actions can be performed at the current time
			actionsEnabled := map[*func(SpecContext)]bool{
				&setDefault:   true,
				&resetDefault: false,
				&setActive:    true,
				&resetActive:  false,
				&done:         false,
			}
			// a table of actions that may become enabled or disabled when each action runs
			actionSideEffects := map[*func(SpecContext)]map[*func(SpecContext)]bool{
				&setDefault: {
					&resetDefault: true,
				},
				&resetDefault: {
					&resetDefault: false,
				},
				&setActive: {
					&resetActive: true,
				},
				&resetActive: {
					&resetActive: false,
				},
				&done: {
					// disable everything
					&setDefault:   false,
					&resetDefault: false,
					&setActive:    false,
					&resetActive:  false,
					&done:         false,
				},
			}
			var defaultStore, activeStore storage.ValueStoreT[T]
			BeforeEach(func() {
				defaultStore = newDefaultStore()
				activeStore = newActiveStore()

				configTracker = driverutil.NewDefaultingConfigTracker(defaultStore, activeStore, setDefaults)

				GinkgoHelper()
				numDefaultWatchEvents = 0
				numActiveWatchEvents = 0
				wctx, wca := context.WithCancel(context.Background())
				var err error
				watchActive, err = activeStore.Watch(wctx)
				Expect(err).NotTo(HaveOccurred())
				watchDefault, err = defaultStore.Watch(wctx)
				Expect(err).NotTo(HaveOccurred())

				watchActive2, err := activeStore.Watch(wctx)
				Expect(err).NotTo(HaveOccurred())
				watchDefault2, err := defaultStore.Watch(wctx)
				Expect(err).NotTo(HaveOccurred())
				actualNumDefaultWatchEvents := 0
				actualNumActiveWatchEvents := 0
				var watches sync.WaitGroup
				watches.Add(2)
				go func() {
					defer watches.Done()
					for range watchDefault2 {
						actualNumDefaultWatchEvents++
					}
				}()
				go func() {
					defer watches.Done()
					for range watchActive2 {
						actualNumActiveWatchEvents++
					}
				}()
				DeferCleanup(func() {
					wca()
					watches.Wait()
					Expect(watchDefault).To(HaveLen(0))
					Expect(watchActive).To(HaveLen(0))
					Expect(actualNumDefaultWatchEvents).To(Equal(numDefaultWatchEvents), "incorrect number of watch events received from default store")
					Expect(actualNumActiveWatchEvents).To(Equal(numActiveWatchEvents), "incorrect number of watch events received from active store")
				})
			})
			random := mathrand.New(mathrand.NewSource(GinkgoRandomSeed()))
			for i := 0; i < 10; i++ {
				Specify(fmt.Sprintf("random actions (%d/10)", i+1), func(ctx SpecContext) {
					minActionsBeforeCancel := 100
					for actionCount := 0; ; actionCount++ {
						// select an action from the list of enabled actions
						enabled := []*func(SpecContext){}
						for action, isEnabled := range actionsEnabled {
							if isEnabled {
								enabled = append(enabled, action)
							}
						}
						if len(enabled) == 0 {
							// no possible actions left
							break
						}
						sort.Slice(enabled, func(i, j int) bool {
							return uintptr(unsafe.Pointer(enabled[i])) < uintptr(unsafe.Pointer(enabled[j]))
						})
						selectedAction := enabled[random.Int()%len(enabled)]
						(*selectedAction)(ctx)

						// update the list of enabled actions based on the side effects of the action that was just run
						for action, enabled := range actionSideEffects[selectedAction] {
							actionsEnabled[action] = enabled
						}
						if actionCount == minActionsBeforeCancel {
							actionsEnabled[&done] = true
						}
					}
				})
			}
		})
	}
}

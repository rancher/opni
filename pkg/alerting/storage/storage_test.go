package storage_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/storage/broker_init"
	"github.com/rancher/opni/pkg/alerting/storage/jetstream"
	"github.com/rancher/opni/pkg/alerting/storage/mem"
	"github.com/rancher/opni/pkg/alerting/storage/opts"
	"github.com/rancher/opni/pkg/test/freeport"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var testTTL = time.Duration(24 * time.Hour)

func ExpectWindowsAreOk(v []*alertingv1.ActiveWindow, neverBefore time.Time) {
	By("Expecting the windows are ok")
	var prevWindow *alertingv1.ActiveWindow
	for _, cur := range v {
		if prevWindow != nil {
			By("expecting the subsequent window starts after the previous window ends")
			Expect(prevWindow.End.AsTime().Before(cur.Start.AsTime())).To(BeTrue())
		}
		By("Expecting the window starts after the cache ttl time")
		Expect(cur.Start.AsTime().After(neverBefore)).To(BeTrue())
		if cur.End != nil {
			By("expecting the window ends after it starts")
			Expect(cur.Start.AsTime().Before(cur.End.AsTime())).To(BeTrue())
		}
		Expect(cur.Fingerprints).NotTo(BeEmpty())
	}
}

type TestAlertStorage[T interfaces.AlertingSecret] interface {
	storage.AlertingStorage[T]
	CheckRedactedSecrets(unredacted, redacted T) bool
}

type TestJetstreamAlertStorage struct {
	storage.AlertingStorage[*testgrpc.TestSecret]
}

type TestJetstreamRouterStore[T routing.OpniRouting] struct {
	storage.RouterStorage
}

func (t *TestJetstreamAlertStorage) CheckRedactedSecrets(unredacted, redacted *testgrpc.TestSecret) bool {
	return unredacted.CheckRedactedSecrets(unredacted, redacted)
}

var _ interfaces.AlertingSecret = (*testgrpc.TestSecret)(nil)

func BuildAlertStorageTestSuite[T interfaces.AlertingSecret](
	name string,
	storageConstructor func() TestAlertStorage[T],
	testcases map[string]T,
) bool {
	return Describe(name, Ordered, Label("integration"), func() {
		ctx, ca := context.WithCancel(context.Background())
		var st TestAlertStorage[T]
		BeforeAll(func() {
			// this closure captures stuff from the suite_test setup,
			// so must live inside a ginkgo node
			st = storageConstructor()
			Expect(testKv).NotTo(BeNil())
			DeferCleanup(func() {
				keys, err := st.ListKeys(ctx)
				if !errors.Is(err, nats.ErrNoKeysFound) {
					Expect(err).NotTo(HaveOccurred())
					for _, key := range keys {
						err = st.Delete(ctx, key)
						Expect(err).NotTo(HaveOccurred())
					}
					ca()
				}
			})
		})
		BeforeEach(func() { // make sure the storage is set before each test
			Expect(st).NotTo(BeNil())
		})
		When("Persisting Alerting Secrets in the AlertStorage", func() {
			It("should redact secrets appropriately", func() {
				for testcaseName, unsecret := range testcases {
					By(fmt.Sprintf("Putting the secret '%s' in the storage", testcaseName))
					err := st.Put(ctx, testcaseName, unsecret)
					Expect(err).NotTo(HaveOccurred())
					By(fmt.Sprintf("Fetching the secret '%s' with redaction enabled", testcaseName))
					redacted, err := st.Get(ctx, testcaseName)
					Expect(err).NotTo(HaveOccurred())
					By(fmt.Sprintf("Checking the secret's '%s' secrets fields are redacted", testcaseName))
					Expect(st.CheckRedactedSecrets(unsecret, redacted)).To(BeTrue())
				}

				By("requesting a list of the secrets in redacted form")
				items, err := st.List(ctx)
				Expect(err).NotTo(HaveOccurred())
				By("Expecting none of the secrets to have unredacted information")
				testcaseRedactedItems := lo.MapToSlice(testcases, func(K string, V T) T {
					V.RedactSecrets()
					return V
				})
				Expect(items).To(ConsistOf(testcaseRedactedItems))
			})

			It("should unredact secrets appropriately", func() {
				for testcaseName, unsecret := range testcases {
					By("Putting the secret in the storage")
					err := st.Put(ctx, testcaseName, unsecret)
					Expect(err).NotTo(HaveOccurred())
					By(fmt.Sprintf("Checking the secret's '%s' secrets fields are unredacted", testcaseName))
					redacted, err := st.Get(ctx, testcaseName, opts.WithUnredacted())
					Expect(err).NotTo(HaveOccurred())
					Expect(unsecret).To(testutil.ProtoEqual(redacted))
				}
				By("requesting a list of the secrets in redacted form")
				items, err := st.List(ctx, opts.WithUnredacted())
				Expect(err).NotTo(HaveOccurred())
				By("Expecting none of the secrets to have redacted information")
				testcaseItems := lo.MapToSlice(testcases, func(K string, V T) T {
					return V
				})
				Expect(items).To(ConsistOf(testcaseItems))
			})
			It("should delete secrets when requested", func() {
				for testcaseName := range testcases {
					By(fmt.Sprintf("Deleting the secret '%s'", testcaseName))
					err := st.Delete(ctx, testcaseName)
					Expect(err).NotTo(HaveOccurred())
				}
				By("verifying that no more secrets are in the storage")
				items, err := st.List(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(items).To(BeEmpty())
			})
		})
	})
}

func BuildAlertingStateCacheTestSuite(
	name string,
	stateCacheConstructor func() storage.AlertingStateCache[*alertingv1.CachedState],
) bool {
	return Describe(name, Ordered, Label("integration"), func() {
		ctx, ca := context.WithCancel(context.Background())
		var cache storage.AlertingStateCache[*alertingv1.CachedState]
		BeforeAll(func() {
			cache = stateCacheConstructor()
			DeferCleanup(func() {
				keys, err := cache.ListKeys(ctx)
				if !errors.Is(err, nats.ErrNoKeysFound) {
					Expect(err).NotTo(HaveOccurred())
					for _, key := range keys {
						err = cache.Delete(ctx, key)
						Expect(err).NotTo(HaveOccurred())
					}
					ca()
				}
			})
		})
		BeforeEach(func() {
			Expect(cache).NotTo(BeNil())
		})

		When("Persisting states in the state cache", func() {
			var oldState *alertingv1.CachedState
			var key string
			It("should not update to the new state if the previous state is the same", func() {
				nowTs := timestamppb.Now()
				oldTs := timestamppb.New(nowTs.AsTime().Add(-time.Minute))
				key = shared.NewAlertingRefId()

				oldState = &alertingv1.CachedState{
					Healthy:   true,
					Firing:    false,
					Timestamp: oldTs,
				}
				newState := util.ProtoClone(oldState)
				newState.Timestamp = nowTs
				Expect(newState.IsEquivalent(oldState)).To(BeTrue())

				By("Putting a fresh state in the cache")
				err := cache.Put(ctx, key, oldState)
				Expect(err).To(Succeed())
				By("checking the last known change matches the timestamp of the fresh state")
				change, err := cache.LastKnownChange(ctx, key)
				Expect(err).To(Succeed())
				Expect(change.AsTime().Unix()).To(BeNumerically("==", oldTs.AsTime().Unix()))

				By("Not overwriting the old state if the new state is equivalent")
				err = cache.Put(ctx, key, newState)
				Expect(err).To(Succeed())
				By("Checking that the old state is still in the cache")
				state, err := cache.Get(ctx, key)
				Expect(err).To(Succeed())
				Expect(state).To(testutil.ProtoEqual(oldState))

				By("Checking the cache still reports the correct last known change")
				lastKnownChangePersisted, err := cache.LastKnownChange(ctx, key)
				Expect(err).To(Succeed())
				Expect(nowTs.AsTime().Unix()).To(BeNumerically(">", oldTs.AsTime().Unix()))
				Expect(lastKnownChangePersisted.AsTime().Unix()).To(BeNumerically("==", oldTs.AsTime().Unix()))
			})

			It("should update to a new state if it is different from the persisted state", func() {
				Expect(oldState).NotTo(BeNil())
				Expect(key).NotTo(BeEmpty())
				ts := timestamppb.Now()
				newState := &alertingv1.CachedState{
					Healthy:   false,
					Firing:    true,
					Timestamp: ts,
				}
				Expect(newState.IsEquivalent(oldState)).To(BeFalse())
				By("Putting the new state in the cache")
				err := cache.Put(ctx, key, newState)
				Expect(err).To(Succeed())
				By("Checking that the new state is in the cache")
				state, err := cache.Get(ctx, key)
				Expect(err).To(Succeed())
				Expect(state).To(testutil.ProtoEqual(newState))
				By("Checking the cache reports the correct last known change")
				lastKnownPersisted, err := cache.LastKnownChange(ctx, key)
				Expect(err).To(Succeed())
				Expect(ts).To(testutil.ProtoEqual(lastKnownPersisted))
			})
			It("should clean up states if requested", func() {
				Expect(key).NotTo(BeEmpty())
				By("Deleting the state")
				err := cache.Delete(ctx, key)
				Expect(err).To(Succeed())
				By("Checking that the state is no longer in the cache")
				_, err = cache.Get(ctx, key)
				Expect(err).To(HaveOccurred())
			})
		})

		When("receiving an unordered batch of messages reporting the same state", func() {
			It("should report the correct last known change", func() {
				key := shared.NewAlertingRefId()
				ts := timestamppb.Now()
				generatorFunc := func(i int) *alertingv1.CachedState {
					return &alertingv1.CachedState{
						Healthy:   true,
						Firing:    false,
						Timestamp: timestamppb.New(ts.AsTime().Add(time.Duration(time.Minute) * time.Duration(i))),
					}
				}
				n := 16
				states := make([]*alertingv1.CachedState, n)
				for i := 0; i < n; i++ {
					states[i] = generatorFunc(i)
				}
				states = lo.Shuffle(states)
				By("Persisting the states in the cache")
				var prevState *alertingv1.CachedState
				for _, state := range states {
					if prevState != nil { // math : equivalency is transitive
						Expect(state.IsEquivalent(prevState)).To(BeTrue())
					}
					state := state
					err := cache.Put(ctx, key, state)
					Expect(err).To(Succeed())
					prevState = state
				}
				By("Checking the cache reports the correct last known change")
				lastKnownChange, err := cache.LastKnownChange(ctx, key)
				Expect(err).To(Succeed())
				Expect(lastKnownChange.AsTime().Unix()).To(BeNumerically("==", ts.AsTime().Unix()))
			})
		})
	})
}

func BuildAlertingIncidentTrackerTestSuite(
	name string,
	incidentTrackerConstructor func() storage.AlertingIncidentTracker[*alertingv1.IncidentIntervals],
) bool {
	return Describe(name, Ordered, Label("integration"), func() {
		ctx, ca := context.WithCancel(context.Background())
		var tracker storage.AlertingIncidentTracker[*alertingv1.IncidentIntervals]
		BeforeAll(func() {
			tracker = incidentTrackerConstructor()
			DeferCleanup(func() {
				keys, err := tracker.ListKeys(ctx)
				if !errors.Is(err, nats.ErrNoKeysFound) {
					Expect(err).NotTo(HaveOccurred())
					for _, key := range keys {
						err = tracker.Delete(ctx, key)
						Expect(err).NotTo(HaveOccurred())
					}
					ca()
				}
			})
		})
		BeforeEach(func() {
			Expect(tracker).NotTo(BeNil())
		})
		When("Using the Alerting Storage Incident Tracker", func() {
			var key string
			It("should open and close incidents when requested", func() {
				ts := timestamppb.Now()
				Expect(tracker).NotTo(BeNil())
				key = shared.NewAlertingRefId()
				fingerprint := shared.NewAlertingRefId()
				generateOption := func() {

					r := rand.Intn(2)
					if r == 0 {
						err := tracker.OpenInterval(ctx, key, fingerprint, timestamppb.Now())
						Expect(err).To(Succeed())
					} else {
						err := tracker.CloseInterval(ctx, key, fingerprint, timestamppb.Now())
						Expect(err).To(Succeed())
					}
				}
				n := 16
				operations := make([]func(), n)
				for i := 0; i < n; i++ {
					operations[i] = generateOption
				}
				operations = lo.Shuffle(operations)
				By("checking we can open and close incidents in random orders without failing")
				for _, operation := range operations {
					operation()
				}
				By("checking the incident tracker is only reporting incident windows")
				windows, err := tracker.GetActiveWindowsFromIncidentTracker(ctx, key, ts, timestamppb.Now())
				Expect(err).To(Succeed())
				Expect(windows).NotTo(HaveLen(0))
				ExpectWindowsAreOk(windows, time.Now().Add(-testTTL))
			})

			It("should evict data that has expired from the cache", func() {
				oldKey := shared.NewAlertingRefId()
				oldFingerprint := shared.NewAlertingRefId()
				startTs := time.Now().Add(-2 * testTTL)
				endTs := time.Now()
				delta := int((endTs.Add(-time.Duration(startTs.Unix()) * time.Second)).Unix())
				n := 16 // number of windows
				for i := 0; i < n; i++ {
					By("checking the incident tracker can open and close intervals")
					newDelta := delta * (i + 1) * n
					start := int(startTs.Unix()) + newDelta
					err := tracker.OpenInterval(ctx, oldKey, oldFingerprint, timestamppb.New(time.Unix(int64(start), 0)))
					Expect(err).To(Succeed())
					end := int(endTs.Unix()) + newDelta*(3/2) // halfway to the next delta offset
					err = tracker.CloseInterval(ctx, oldKey, oldFingerprint, timestamppb.New(time.Unix(int64(end), 0)))
					Expect(err).To(Succeed())
				}
				By("checking the incident tracker is only reporting incident windows")
				windows, err := tracker.GetActiveWindowsFromIncidentTracker(ctx, oldKey, timestamppb.Now(), timestamppb.Now())
				Expect(err).To(Succeed())
				Expect(windows).NotTo(HaveLen(0))
				Expect(len(windows)).To(BeNumerically("<=", n))
				ExpectWindowsAreOk(windows, endTs.Add(-testTTL))
			})
		})
	})
}

func BuildAlertRouterStorageTestSuite(
	name string,
	routerStoreConstructor func() storage.RouterStorage,
	defaultRouter routing.OpniRouting,
) bool {
	return Describe(name, Ordered, Label("integration"), func() {
		ctx, ca := context.WithCancel(context.Background())
		var routerStore storage.RouterStorage
		BeforeAll(func() {
			routerStore = routerStoreConstructor()
			DeferCleanup(func() {
				ca()
			})
		})
		BeforeEach(func() {
			Expect(routerStore).NotTo(BeNil())
		})

		var originalRouter routing.OpniRouting

		When("we want to store a router object", func() {
			It("should be able to put a default router and get it back", func() {
				By("putting it in the object store")
				originalRouter = defaultRouter
				err := routerStore.Put(ctx, "cluster1", defaultRouter)
				Expect(err).To(Succeed())
				By("getting it back from the object store")
				getRouter, err := routerStore.Get(ctx, "cluster1")
				Expect(err).To(Succeed())
				Expect(getRouter).NotTo(BeNil())
				By("checking the router object is the same")
				test.ExpectRouterEqual(getRouter, defaultRouter)
			})
		})

		When("we want to update a router object", func() {
			It("should persist the update", func() {
				Expect(originalRouter).NotTo(BeNil())
				By("getting the router object from the object store")
				getRouter, err := routerStore.Get(ctx, "cluster1")
				Expect(err).To(Succeed())
				Expect(originalRouter).NotTo(BeNil())
				By("attaching some configurations to the obtained router")
				endpSet := test.CreateRandomSetOfEndpoints()
				getRouter.SetDefaultNamespaceConfig(lo.Map(
					lo.Samples(
						lo.Values(endpSet),
						len(endpSet)-2,
					),
					func(v *alertingv1.FullAttachedEndpoint, _ int) *alertingv1.AlertEndpoint {
						return v.AlertEndpoint
					}))
				By("expecting the modified router to be different from the original")
				test.ExpectRouterNotEqual(getRouter, originalRouter)

				err = routerStore.Put(ctx, "cluster1", getRouter)
				Expect(err).To(Succeed())
				updatedRouter, err := routerStore.Get(ctx, "cluster1")
				Expect(err).To(Succeed())
				Expect(updatedRouter).NotTo(BeNil())
				By("expecting the persisted updated router to be different from the original")
				test.ExpectRouterNotEqual(updatedRouter, originalRouter)
			})
		})

	})
}

func BuildStorageClientSetSuite(
	name string,
	brokerConstructor func() storage.AlertingStoreBroker,
) bool {
	return Describe(name, Ordered, Label("integration"), func() {
		var s storage.AlertingClientSet
		var ctx context.Context
		BeforeAll(func() {
			broker := broker_init.NewDefaultAlertingBroker(embeddedJetstream)

			s = broker.NewClientSet()
			Expect(s).NotTo(BeNil())
			ctx = env.Context()
			Expect(env).NotTo(BeNil())
			Expect(ctx).NotTo(BeNil())
			DeferCleanup(func() {
				s.Purge(ctx)
			})
		})

		When("initializing the hash ring", func() {
			syncOpts := opts.NewSyncOptions()
			Specify("the hash should be empty unless it is explicitly requested to calculate it", func() {
				hash := s.GetHash(ctx, shared.SingleConfigId)
				Expect(hash).To(BeEmpty())

				err := s.CalculateHash(ctx, shared.SingleConfigId, syncOpts)
				Expect(err).To(Succeed())
			})

			Specify("the hash ring should change its hash when configurations change enough to warrant an update", func() {
				id1 := uuid.New().String()
				id2 := uuid.New().String()
				mutateState := []func(){
					func() { // new
						err := s.Conditions().Put(ctx, id1, &alertingv1.AlertCondition{
							Name:        "sample condition",
							Description: "sample description",
							Id:          id1,
							LastUpdated: timestamppb.Now(),
							Severity:    alertingv1.OpniSeverity_Info,
						})
						Expect(err).To(Succeed())
					},
					func() { // new
						err := s.Conditions().Put(ctx, id2, &alertingv1.AlertCondition{
							Name:        "sample condition",
							Description: "sample description",
							Id:          id2,
							LastUpdated: timestamppb.Now(),
							Severity:    alertingv1.OpniSeverity_Info,
						})
						Expect(err).To(Succeed())
					},
					func() { //update timestamp
						err := s.Conditions().Put(ctx, id2, &alertingv1.AlertCondition{
							Name:        "sample condition",
							Description: "sample description",
							Id:          id2,
							LastUpdated: timestamppb.Now(),
							Severity:    alertingv1.OpniSeverity_Info,
						})
						Expect(err).To(Succeed())
					},
					func() {
						err := s.Endpoints().Put(ctx, id1, &alertingv1.AlertEndpoint{
							Name:        "sample endpoint",
							Description: "sample description",
							Id:          id1,
							LastUpdated: timestamppb.Now(),
						})
						Expect(err).To(Succeed())
					},
					func() {
						err := s.Endpoints().Put(ctx, id2, &alertingv1.AlertEndpoint{
							Name:        "sample endpoint",
							Description: "sample description",
							Id:          id2,
							LastUpdated: timestamppb.Now(),
						})
						Expect(err).To(Succeed())

					},
				}
				for _, f := range mutateState {
					oldHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					f()
					err := s.CalculateHash(ctx, shared.SingleConfigId, syncOpts)
					Expect(err).To(Succeed())
					newHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					Expect(newHash).NotTo(Equal(oldHash))
				}
			})

			Specify("the default caching endpoint changing should trigger a hash change", func() {
				for i := 0; i < 10; i++ {
					oldHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					syncOpts.DefaultEndpoint = uuid.New().String()
					err := s.CalculateHash(ctx, shared.SingleConfigId, syncOpts)
					Expect(err).To(Succeed())
					newHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					Expect(newHash).NotTo(Equal(oldHash))
				}
			})

			Specify("the hash should not change when no meaningul configuration change occurs", func() {
				for i := 0; i < 10; i++ {
					oldHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					err := s.CalculateHash(ctx, shared.SingleConfigId, syncOpts)
					Expect(err).To(Succeed())
					newHash := strings.Clone(s.GetHash(ctx, shared.SingleConfigId))
					Expect(newHash).To(Equal(oldHash))
				}
			})
		})

		When("we request a data purge", func() {
			Specify("there should be no ", func() {
				err := s.Purge(ctx)
				Expect(err).To(Succeed())
				By("checking that the condition store is empty")
				conds, err := s.Conditions().List(ctx)
				Expect(err).To(Succeed())
				Expect(conds).To(BeEmpty())

				By("checking that the endpoint store is empty")
				endps, err := s.Endpoints().List(ctx)
				Expect(err).To(Succeed())
				Expect(endps).To(BeEmpty())

				By("checking that the state cache is empty")
				cache, err := s.States().List(ctx)
				Expect(err).To(Succeed())
				Expect(cache).To(BeEmpty())

				By("checking that the incident cache is empty")
				incs, err := s.Incidents().List(ctx)
				Expect(err).To(Succeed())
				Expect(incs).To(BeEmpty())

				By("checking that the virtual config store is empty")
				vcs, err := s.Routers().List(ctx)
				Expect(err).To(Succeed())
				Expect(vcs).To(BeEmpty())
			})
		})

		When("force syncing with no user configurations", func() {
			It("should create a default routing tree that is valid", func() {
				err := s.ForceSync(ctx)
				Expect(err).To(Succeed())
				By("getting the routing tree")
				tree, err := s.Routers().Get(ctx, shared.SingleConfigId)
				Expect(err).To(Succeed())
				Expect(tree).NotTo(BeNil())
				By("checking that it can build an Alertmanager configuration")
				cfg, err := tree.BuildConfig()
				Expect(err).To(Succeed())
				newDir := env.GenerateNewTempDirectory("force-sync")
				test.ExpectAlertManagerConfigToBeValid(
					env,
					newDir,
					"default-after-force-sync.yaml",
					ctx,
					cfg,
					freeport.GetFreePort(),
				)
			})
		})

		When("force syncing with user configurations", func() {
			It("should create a routing tree with the correct configuration", func() {
				endps := test.CreateRandomSetOfEndpoints()
				for _, endp := range endps {
					err := s.Endpoints().Put(ctx, endp.GetEndpointId(), endp.GetAlertEndpoint())
					Expect(err).To(Succeed())
				}

				sampleAttachedEndpoints := lo.Samples(
					lo.Map(lo.Values(endps),
						func(a *alertingv1.FullAttachedEndpoint, _ int) *alertingv1.AttachedEndpoint {
							return &alertingv1.AttachedEndpoint{
								EndpointId: a.EndpointId,
							}
						},
					),
					rand.Intn(len(endps))+1,
				)

				conditionId := uuid.New().String()
				err := s.Conditions().Put(ctx, conditionId, &alertingv1.AlertCondition{
					Name:        "sample condition",
					Description: "sample condition",
					Severity:    alertingv1.OpniSeverity_Info,
					Id:          conditionId,
					AlertType: &alertingv1.AlertTypeDetails{
						Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
							PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
								ClusterId: &corev1.Reference{
									Id: "cluster1",
								},
								Query: "up==0",
								For:   durationpb.New(time.Minute),
							},
						},
					},
					AttachedEndpoints: &alertingv1.AttachedEndpoints{
						Items: sampleAttachedEndpoints,
						Details: &alertingv1.EndpointImplementation{
							Title: "test",
							Body:  "test-body",
						},
					},
				})
				Expect(err).To(Succeed())
				err = s.ForceSync(ctx)
				Expect(err).To(Succeed())
				By("getting the routing tree")
				tree, err := s.Routers().Get(ctx, shared.SingleConfigId)
				Expect(err).To(Succeed())
				Expect(tree).NotTo(BeNil())
				By("checking that it can build an Alertmanager configuration")
				cfg, err := tree.BuildConfig()
				Expect(err).To(Succeed())
				newDir := env.GenerateNewTempDirectory("force-sync")
				test.ExpectAlertManagerConfigToBeValid(
					env,
					newDir,
					"user-configs-force-sync.yaml",
					ctx,
					cfg,
					freeport.GetFreePort(),
				)
			})
		})

		When("an internal datasource uses incident & state caches", func() {
			It("should persist states", func() {
				err := s.States().Put(ctx, "test", &alertingv1.CachedState{})
				Expect(err).To(Succeed())

			})

			It("should persist incidents", func() {
				err := s.Incidents().Put(ctx, "test", &alertingv1.IncidentIntervals{})
				Expect(err).To(Succeed())
			})
		})
	})
}

//var _ = BuildAlertRouterStorageTestSuite(
//	"Alerting RouterV1 Jetstream Storage Test",
//	func() storage.RouterStorage[*routing.OpniRouterV1] {
//		return &TestJetstreamRouterStore[*routing.OpniRouterV1]{
//			RouterStorage: storage.NewJetstreamRouterStore(testObj, "/router"),
//		}
//	},
//	routing.NewOpniRouterV1("http://localhost:3000"),
//)

var _ = BuildAlertRouterStorageTestSuite(
	"Alerting Router in memory store",
	func() storage.RouterStorage {
		return mem.NewInMemoryRouterStore()
	},
	routing.NewDefaultOpniRouting(),
)

var _ = BuildAlertStorageTestSuite(
	"Alerting JetStream Storage Test Secret",
	func() TestAlertStorage[*testgrpc.TestSecret] {
		return &TestJetstreamAlertStorage{
			AlertingStorage: jetstream.NewJetStreamAlertingStorage[*testgrpc.TestSecret](testKv, "/testsecret"),
		}
	},
	map[string]*testgrpc.TestSecret{
		"simple": {
			Username: "test",
			Password: "dog124",
		},
	},
)

var _ = BuildAlertingStateCacheTestSuite(
	"Alerting State Cache Jetstream Cache",
	func() storage.AlertingStateCache[*alertingv1.CachedState] {
		return jetstream.NewJetStreamAlertingStateCache(testKv2, "/statecache")
	},
)

var _ = BuildAlertingIncidentTrackerTestSuite(
	"Alerting Incident Tracker Jetstream Cache",
	func() storage.AlertingIncidentTracker[*alertingv1.IncidentIntervals] {
		return jetstream.NewJetStreamAlertingIncidentTracker(testKv2, "/incidenttracker", testTTL)
	},
)

var _ = BuildStorageClientSetSuite(
	"Default Storage clientset hash ring & syncing",
	func() storage.AlertingStoreBroker {
		return broker_init.NewDefaultAlertingBroker(embeddedJetstream)
	},
)

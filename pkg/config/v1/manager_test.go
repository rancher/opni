package configv1_test

import (
	context "context"
	errors "errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fieldmask"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/pkg/util/pathreflect"
	"github.com/rancher/opni/pkg/util/protorand"
)

// Note that this test is run in the v1 package, not v1_test, since we need
// reflect access to unexported types.

var _ = Describe("Gateway Config Manager", Label("unit"), func() {
	var mgr *configv1.GatewayConfigManager
	defaultStore := inmemory.NewValueStore[*configv1.GatewayConfigSpec](util.ProtoClone)
	activeStore := inmemory.NewValueStore[*configv1.GatewayConfigSpec](util.ProtoClone)

	BeforeEach(func() {
		mgr = configv1.NewGatewayConfigManager(defaultStore, activeStore, flagutil.LoadDefaults)
		ctx, ca := context.WithCancel(context.Background())
		Expect(mgr.Start(ctx)).To(Succeed())
		DeferCleanup(ca)
	})

	It("should create reactive messages", func(ctx SpecContext) {
		msg := &configv1.GatewayConfigSpec{}
		rand := protorand.New[*configv1.GatewayConfigSpec]()
		rand.ExcludeMask(&fieldmaskpb.FieldMask{
			Paths: []string{
				"revision",
			},
		})
		rand.Seed(GinkgoRandomSeed())

		By("creating reactive messages for every possible path")
		allPaths := pathreflect.AllPaths(msg)
		reactiveMsgs := make([]reactive.Value, len(allPaths))

		verifyWatches := func(spec *configv1.GatewayConfigSpec, ws []<-chan protoreflect.Value, pathsToCheck ...map[string]struct{}) {
			recvFailures := []error{}
		ALL_PATHS:
			for i := 0; i < len(allPaths); i++ {
				path := allPaths[i]
				rm := reactiveMsgs[i]
				w := ws[i]

				if strings.HasPrefix(path.String(), "(config.v1.GatewayConfigSpec).revision") {
					// ignore the revision field; a reactive message for it has undefined behavior
					select {
					case <-w:
					default:
					}
					continue
				}

				if len(pathsToCheck) > 0 {
					if _, ok := pathsToCheck[0][path[1:].String()[1:]]; !ok {
						Expect(w).NotTo(Receive(), "expected not to receive an update for path %s", path)
						continue
					}
				}

				var v protoreflect.Value
			RECV:
				for i := 0; i < 10; i++ {
					select {
					case v = <-w:
						break RECV
					default:
						time.Sleep(10 * time.Millisecond)
					}
					if i == 9 {
						recvFailures = append(recvFailures, errors.New("did not receive an update for path "+path.String()))
						continue ALL_PATHS
					}
				}
				var actual protoreflect.Value
				if spec == nil {
					actual = protoreflect.ValueOf(nil)
				} else {
					actual = pathreflect.Value(spec, path)
				}
				Expect(v).To(testutil.ProtoValueEqual(rm.Value()))
				Expect(v).To(testutil.ProtoValueEqual(actual))
			}

			Expect(errors.Join(recvFailures...)).To(BeNil())

			for _, c := range ws {
				Expect(c).To(HaveLen(0), "expected all watchers to be read")
			}
		}

		watches := make([]<-chan protoreflect.Value, len(allPaths))
		for i, path := range allPaths {
			rm := mgr.Reactive(path)
			reactiveMsgs[i] = rm

			c := rm.Watch(ctx)
			watches[i] = c

			Expect(len(c)).To(BeZero())
		}

		By("setting all fields in the spec to random values")
		spec := rand.MustGen()
		_, err := mgr.SetConfiguration(ctx, &configv1.SetRequest{
			Spec: spec,
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying that all reactive messages received an update")
		verifyWatches(spec, watches)

		By("adding a second watch to each reactive message")
		watches2 := make([]<-chan protoreflect.Value, len(watches))

		for i, rm := range reactiveMsgs {
			watches2[i] = rm.Watch(ctx)
		}

		By("verifying that new watches receive the current value")
		verifyWatches(spec, watches2)

		By("modifying all fields in the spec")
		spec2 := rand.MustGen()
		_, err = mgr.SetConfiguration(ctx, &configv1.SetRequest{
			Spec: spec2,
		})
		Expect(err).NotTo(HaveOccurred())

		By("verifying that both watches received an update")
		// some fields have a limited set of possible values
		updatedFields := fieldmask.Diff(spec, spec2).Paths
		pathsToCheck := map[string]struct{}{}
		for _, path := range updatedFields {
			parts := strings.Split(path, ".")
			for i := range parts {
				pathsToCheck[strings.Join(parts[:i+1], ".")] = struct{}{}
			}
		}
		verifyWatches(spec2, watches2, pathsToCheck)
		verifyWatches(spec2, watches, pathsToCheck)

		By("deleting the configuration")
		err = mgr.Tracker().ResetConfig(ctx, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that all reactive messages received an update")
		verifyWatches(nil, watches2)
		verifyWatches(nil, watches)
	})

})

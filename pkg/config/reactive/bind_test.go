package reactive_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/rancher/opni/pkg/config/reactive"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/flagutil"
)

var _ = Describe("Bind", Label("unit"), func() {
	var ctrl *reactive.Controller[*ext.SampleConfiguration]
	var defaultStore, activeStore storage.ValueStoreT[*ext.SampleConfiguration]

	BeforeEach(func() {
		defaultStore = inmemory.NewValueStore[*ext.SampleConfiguration](util.ProtoClone)
		activeStore = inmemory.NewValueStore[*ext.SampleConfiguration](util.ProtoClone)
		ctrl = reactive.NewController(driverutil.NewDefaultingConfigTracker(defaultStore, activeStore, flagutil.LoadDefaults))
		ctx, ca := context.WithCancel(context.Background())
		Expect(ctrl.Start(ctx)).To(Succeed())
		DeferCleanup(ca)
	})

	It("should bind reactive values", func(ctx SpecContext) {
		called := make(chan struct{})
		reactive.Bind(ctx,
			func(v []protoreflect.Value) {
				defer close(called)
				Expect(v).To(HaveLen(6))
				Expect(v[0].Int()).To(Equal(int64(100)))
				Expect(v[1].Int()).To(Equal(int64(200)))
				Expect(v[2].Int()).To(Equal(int64(300)))
				Expect(v[3].Int()).To(Equal(int64(400)))
				Expect(v[4].Int()).To(Equal(int64(500)))
				Expect(v[5].Int()).To(Equal(int64(600)))
			},
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field1()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field2()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field3()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field4()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field5()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field6()),
		)

		Expect(activeStore.Put(ctx, &ext.SampleConfiguration{
			MessageField: &ext.SampleMessage{
				Field6: &ext.Sample6FieldMsg{
					Field1: 100,
					Field2: 200,
					Field3: 300,
					Field4: 400,
					Field5: 500,
					Field6: 600,
				},
			},
		})).To(Succeed())

		Eventually(called).Should(BeClosed())
		// ensure no more updates are received
		Consistently(called).Should(BeClosed())
	})

	It("should handle partial updates", func(ctx SpecContext) {
		callback := new(func(v []protoreflect.Value))
		reactive.Bind(ctx,
			func(v []protoreflect.Value) {
				(*callback)(v)
			},
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field1()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field2()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field3()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field4()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field5()),
			ctrl.Reactive((&ext.SampleConfiguration{}).ProtoPath().MessageField().Field6().Field6()),
		)

		called := make(chan struct{})
		*callback = func(v []protoreflect.Value) {
			defer close(called)
			Expect(v).To(HaveLen(6))
			Expect(v[0].Int()).To(Equal(int64(100)))
			Expect(v[1].Int()).To(Equal(int64(200)))
			Expect(v[2].Int()).To(Equal(int64(300)))
			Expect(v[3].Int()).To(Equal(int64(400)))
			Expect(v[4].Int()).To(Equal(int64(500)))
			Expect(v[5].Int()).To(Equal(int64(600)))
		}

		Expect(activeStore.Put(ctx, &ext.SampleConfiguration{
			MessageField: &ext.SampleMessage{
				Field6: &ext.Sample6FieldMsg{
					Field1: 100,
					Field2: 200,
					Field3: 300,
					Field4: 400,
					Field5: 500,
					Field6: 600,
				},
			},
		})).To(Succeed())

		Eventually(called).Should(BeClosed())
		Consistently(called).Should(BeClosed())

		called = make(chan struct{})
		*callback = func(v []protoreflect.Value) {
			defer close(called)
			Expect(v).To(HaveLen(6))
			Expect(v[0].Int()).To(Equal(int64(1000)))
			Expect(v[1].Int()).To(Equal(int64(2000)))
			Expect(v[2].Int()).To(Equal(int64(3000)))
			Expect(v[3].Int()).To(Equal(int64(400)))
			Expect(v[4].Int()).To(Equal(int64(500)))
			Expect(v[5].Int()).To(Equal(int64(600)))
		}

		Expect(activeStore.Put(ctx, &ext.SampleConfiguration{
			MessageField: &ext.SampleMessage{
				Field6: &ext.Sample6FieldMsg{
					Field1: 1000,
					Field2: 2000,
					Field3: 3000,
					Field4: 400,
					Field5: 500,
					Field6: 600,
				},
			},
		})).To(Succeed())

		Eventually(called).Should(BeClosed())
		Consistently(called).Should(BeClosed())
	})
})

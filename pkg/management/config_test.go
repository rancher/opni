package management_test

import (
	"context"
	"encoding/json"

	"github.com/alecthomas/jsonschema"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/plugins"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Config", Ordered, Label("unit", "slow"), func() {
	var tv *testVars
	var sampleObjects []meta.Object
	var lifecycler config.Lifecycler

	BeforeAll(func() {
		sampleObjects = []meta.Object{
			&v1beta1.GatewayConfig{
				TypeMeta: meta.TypeMeta{
					Kind:       "GatewayConfig",
					APIVersion: "v1beta1",
				},
				Spec: v1beta1.GatewayConfigSpec{
					GRPCListenAddress: "foo",
					HTTPListenAddress: "bar",
					Management: v1beta1.ManagementSpec{
						GRPCListenAddress: "bar",
						HTTPListenAddress: "baz",
					},
				},
			},
		}
		lifecycler = config.NewLifecycler(sampleObjects)
		setupManagementServer(&tv, plugins.NoopLoader, management.WithLifecycler(lifecycler))()
	})

	It("should retrieve the current config", func() {
		docs, err := tv.client.GetConfig(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		objects := meta.ObjectList{}
		for _, doc := range docs.Documents {
			obj, err := config.LoadObject(doc.Json)
			Expect(err).NotTo(HaveOccurred())
			objects = append(objects, obj)
		}
		ok := false
		objects.Visit(func(obj *v1beta1.GatewayConfig, schema *jsonschema.Schema) {
			ok = true
			Expect(obj).To(Equal(sampleObjects[0]))
		})
		Expect(ok).To(BeTrue())
	})
	It("should update the config", func() {
		newObj := &v1beta1.GatewayConfig{
			TypeMeta: meta.TypeMeta{
				Kind:       "GatewayConfig",
				APIVersion: "v1beta1",
			},
			Spec: v1beta1.GatewayConfigSpec{
				GRPCListenAddress: "foo2",
			},
		}
		doc, err := json.Marshal(newObj)
		Expect(err).NotTo(HaveOccurred())
		reloadC, err := lifecycler.ReloadC()
		Expect(err).NotTo(HaveOccurred())

		go func() {
			<-reloadC
		}()
		_, err = tv.client.UpdateConfig(context.Background(), &managementv1.UpdateConfigRequest{
			Documents: []*managementv1.ConfigDocument{
				{
					Json: doc,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		docs, err := tv.client.GetConfig(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		objects := meta.ObjectList{}
		for _, doc := range docs.Documents {
			obj, err := config.LoadObject(doc.Json)
			Expect(err).NotTo(HaveOccurred())
			objects = append(objects, obj)
		}
		ok := false
		objects.Visit(func(obj *v1beta1.GatewayConfig, schema *jsonschema.Schema) {
			ok = true
			Expect(obj).To(Equal(newObj))
		})
		Expect(ok).To(BeTrue())
	})
})

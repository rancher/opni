package management_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jhump/protoreflect/grpcreflect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/samber/lo"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	mock_apiextensions "github.com/rancher/opni/pkg/test/mock/apiextensions"
	mock_ext "github.com/rancher/opni/pkg/test/mock/ext"
	"github.com/rancher/opni/pkg/test/testdata/plugins/ext"
)

var _ = Describe("Extensions", Ordered, Label("slow"), func() {
	var tv *testVars
	var descriptorLogic func() (*apiextensions.ServiceDescriptorProtoList, error)
	shouldLoadExt1 := atomic.NewBool(true)
	shouldLoadExt2 := atomic.NewBool(false)
	JustBeforeEach(func() {
		tv = &testVars{}
		pl := plugins.NewPluginLoader()
		tv.ctrl = gomock.NewController(GinkgoT())
		extSrv := mock_ext.NewMockExtServer(tv.ctrl)
		extSrv.EXPECT().
			Foo(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
				return &ext.FooResponse{
					Response: strings.ToUpper(req.Request),
				}, nil
			}).
			AnyTimes()
		extSrv.EXPECT().
			Bar(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.BarRequest) (*ext.BarResponse, error) {
				return &ext.BarResponse{
					Param1: req.Param1,
					Param2: req.Param2,
					Param3: req.Param3,
				}, nil
			}).
			AnyTimes()
		extSrv.EXPECT().
			Baz(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.BazRequest) (*ext.BazRequest, error) {
				return req, nil
			}).
			AnyTimes()
		extSrv.EXPECT().
			Set(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.SetRequest) (*ext.SetRequest, error) {
				return req, nil
			}).
			AnyTimes()
		extSrv.EXPECT().
			ServerStream(gomock.Any(), gomock.Any()).
			DoAndReturn(func(req *ext.FooRequest, stream ext.Ext_ServerStreamServer) (*emptypb.Empty, error) {
				stream.SendHeader(metadata.Pairs("foo", "header"))
				stream.SetTrailer(metadata.Pairs("foo", "trailer"))
				for i := 0; i < 10; i++ {
					if err := stream.Send(&ext.FooResponse{
						Response: strings.ToUpper(req.Request),
					}); err != nil {
						return nil, err
					}
				}
				return &emptypb.Empty{}, nil
			}).
			AnyTimes()
		extSrv.EXPECT().
			ClientStream(gomock.Any()).
			DoAndReturn(func(stream ext.Ext_ClientStreamServer) error {
				stream.SendHeader(metadata.Pairs("foo", "header"))
				stream.SetTrailer(metadata.Pairs("foo", "trailer"))
				var requests []string
				for {
					req, err := stream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						return err
					}
					requests = append(requests, req.Request)
				}
				return stream.SendAndClose(&ext.FooResponse{
					Response: strings.Join(requests, ","),
				})
			}).
			AnyTimes()

		ext2Srv := mock_ext.NewMockExt2Server(tv.ctrl)
		ext2Srv.EXPECT().
			Foo(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *ext.FooRequest) (*ext.FooResponse, error) {
				return &ext.FooResponse{
					Response: strings.ToLower(req.Request),
				}, nil
			}).
			AnyTimes()

		// Create the management server, which installs hooks into the plugin loader
		setupManagementServer(&tv, pl)()

		// Loading the plugins after installing the hooks ensures LoadOne will block
		// until all hooks return.
		if shouldLoadExt1.Load() {
			apiextSrv := &mock_apiextensions.MockManagementAPIExtensionServerImpl{
				MockManagementAPIExtensionServer: mock_apiextensions.NewMockManagementAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor, err := grpcreflect.LoadServiceDescriptor(&ext.Ext_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv.EXPECT().
				Descriptors(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*apiextensions.ServiceDescriptorProtoList, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor.GetFullyQualifiedName()
					sd := serviceDescriptor.AsServiceDescriptorProto()
					sd.Name = &fqn
					return &apiextensions.ServiceDescriptorProtoList{
						Items: []*descriptorpb.ServiceDescriptorProto{sd},
					}, nil
				})
			cc := test.NewApiExtensionTestPlugin(apiextSrv, &ext.Ext_ServiceDesc, &mock_ext.MockExtServerImpl{
				MockExtServer: extSrv,
			})
			pl.LoadOne(context.Background(), meta.PluginMeta{
				BinaryPath: "test1",
				GoVersion:  "test1",
				Module:     "test1",
			}, cc)
		}

		if shouldLoadExt2.Load() {
			apiextSrv2 := &mock_apiextensions.MockManagementAPIExtensionServerImpl{
				MockManagementAPIExtensionServer: mock_apiextensions.NewMockManagementAPIExtensionServer(tv.ctrl),
			}
			serviceDescriptor2, err := grpcreflect.LoadServiceDescriptor(&ext.Ext2_ServiceDesc)
			Expect(err).NotTo(HaveOccurred())
			apiextSrv2.EXPECT().
				Descriptors(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, *emptypb.Empty) (*apiextensions.ServiceDescriptorProtoList, error) {
					if descriptorLogic != nil {
						return descriptorLogic()
					}
					fqn := serviceDescriptor2.GetFullyQualifiedName()
					sd := serviceDescriptor2.AsServiceDescriptorProto()
					sd.Name = &fqn
					return &apiextensions.ServiceDescriptorProtoList{
						Items: []*descriptorpb.ServiceDescriptorProto{sd},
					}, nil
				})
			cc2 := test.NewApiExtensionTestPlugin(apiextSrv2, &ext.Ext2_ServiceDesc, &mock_ext.MockExt2ServerImpl{
				MockExt2Server: ext2Srv,
			})
			pl.LoadOne(context.Background(), meta.PluginMeta{
				BinaryPath: "test2",
				GoVersion:  "test2",
				Module:     "test2",
			}, cc2)
		}

		pl.Complete()
	})
	It("should load API extensions", func() {
		extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		Expect(extensions.Items).To(HaveLen(1))
		Expect(extensions.Items[0].ServiceDesc.GetName()).To(Equal("Ext"))
		Expect(extensions.Items[0].ServiceDesc.Method[0].GetName()).To(Equal("Foo"))
		Expect(extensions.Items[0].ServiceDesc.Method[1].GetName()).To(Equal("Bar"))
		Expect(extensions.Items[0].ServiceDesc.Method[2].GetName()).To(Equal("Baz"))
		Expect(extensions.Items[0].ServiceDesc.Method[3].GetName()).To(Equal("Set"))
		Expect(extensions.Items[0].ServiceDesc.Method[4].GetName()).To(Equal("ServerStream"))
		Expect(extensions.Items[0].ServiceDesc.Method[5].GetName()).To(Equal("ClientStream"))
		Expect(extensions.Items[0].Rules).To(HaveLen(12))
		Expect(extensions.Items[0].Rules[0].Http.GetPost()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[0].Http.GetBody()).To(Equal("request"))
		Expect(extensions.Items[0].Rules[1].Http.GetGet()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetPut()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[2].Http.GetBody()).To(Equal("request"))
		Expect(extensions.Items[0].Rules[3].Http.GetDelete()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetPatch()).To(Equal("/foo"))
		Expect(extensions.Items[0].Rules[4].Http.GetBody()).To(Equal("request"))
		Expect(extensions.Items[0].Rules[5].Http.GetPost()).To(Equal("/bar/{param1}/{param2}"))
		Expect(extensions.Items[0].Rules[5].Http.GetBody()).To(Equal("param3"))
		Expect(extensions.Items[0].Rules[6].Http.GetGet()).To(Equal("/bar/{param1}/{param2}/{param3}"))
		Expect(extensions.Items[0].Rules[7].Http.GetPost()).To(Equal("/baz"))
		Expect(extensions.Items[0].Rules[8].Http.GetPost()).To(Equal("/baz/{paramMsg.paramBool}/{paramMsg.paramString}/{paramMsg.paramEnum}"))
		Expect(extensions.Items[0].Rules[9].Http.GetPost()).To(Equal("/baz/{paramMsg.paramMsg.paramMsg.paramMsg.paramString}"))
		Expect(extensions.Items[0].Rules[10].Http.GetPut()).To(Equal("/set/{node.id}"))
		Expect(extensions.Items[0].Rules[11].Http.GetPut()).To(Equal("/set/example/{node.id}"))
	})
	It("should forward gRPC calls to the plugin", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()
		client := ext.NewExtClient(cc)
		resp, err := client.Foo(context.Background(), &ext.FooRequest{
			Request: "hello",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Response).To(Equal("HELLO"))
	})
	It("should forward HTTP calls to the plugin", func() {
		Eventually(func() error {
			resp, err := http.Post(tv.httpEndpoint+"/Ext/foo",
				"application/json", strings.NewReader("hello"))
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("expecte response status code 200, got : %d", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			resp.Body.Close()
			expected := `{"response":"HELLO"}`
			if string(body) != expected {
				return fmt.Errorf("Expected response body %s to equal %s", string(body), expected)
			}
			return nil
		}, time.Second*3, time.Millisecond*20).Should(Succeed())
	})
	It("should forward HTTP calls containing path parameters to the plugin", func() {
		tries := 10 // need to wait a bit for the server to become ready
		for {
			resp, err := http.Post(tv.httpEndpoint+"/Ext/bar/a/b",
				"application/json", strings.NewReader("c"))
			if (err != nil || resp.StatusCode != 200) && tries > 0 {
				tries--
				time.Sleep(100 * time.Millisecond)
				continue
			}
			By("testing valid requests")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(string(body)).To(Equal(`{"param1":"a","param2":"b","param3":"c"}`))

			resp, err = http.Get(tv.httpEndpoint + "/Ext/bar/a/b/c")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			body, err = io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(string(body)).To(Equal(`{"param1":"a","param2":"b","param3":"c"}`))

			{
				resp, err = http.Post(tv.httpEndpoint+"/Ext/baz/true/asdf/BAR", "application/json", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				body, err = io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				var bazReq ext.BazRequest
				Expect(protojson.Unmarshal(body, &bazReq)).To(Succeed())
				Expect(&bazReq).To(testutil.ProtoEqual(&ext.BazRequest{
					ParamMsg: &ext.BazRequest{
						ParamBool:   true,
						ParamString: "asdf",
						ParamEnum:   ext.BazRequest_BAR,
					},
				}))
			}

			{
				resp, err = http.Post(tv.httpEndpoint+"/Ext/baz/testing", "application/json", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				body, err = io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				var bazReq ext.BazRequest
				Expect(protojson.Unmarshal(body, &bazReq)).To(Succeed())
				Expect(&bazReq).To(testutil.ProtoEqual(&ext.BazRequest{
					ParamMsg: &ext.BazRequest{
						ParamMsg: &ext.BazRequest{
							ParamMsg: &ext.BazRequest{
								ParamMsg: &ext.BazRequest{
									ParamString: "testing",
								},
							},
						},
					},
				}))
			}

			{
				req, err := http.NewRequest(http.MethodPut, tv.httpEndpoint+"/Ext/set/testing1", strings.NewReader(`{"value": "value1"}`))
				Expect(err).NotTo(HaveOccurred())
				resp, err = http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				body, err = io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				var setReq ext.SetRequest
				Expect(protojson.Unmarshal(body, &setReq)).To(Succeed())
				Expect(&setReq).To(testutil.ProtoEqual(&ext.SetRequest{
					Node: &ext.Reference{
						Id: "testing1",
					},
					Value: "value1",
				}))
			}

			{
				req, err := http.NewRequest(http.MethodPut, tv.httpEndpoint+"/Ext/set/example/testing1", strings.NewReader(`{"value": "value1"}`))
				Expect(err).NotTo(HaveOccurred())
				resp, err = http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				body, err = io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				var setReq ext.SetRequest
				Expect(protojson.Unmarshal(body, &setReq)).To(Succeed())
				Expect(&setReq).To(testutil.ProtoEqual(&ext.SetRequest{
					Node: &ext.Reference{
						Id: "testing1",
					},
					Example: &ext.ExampleValue{
						Value: "value1",
					},
				}))
			}

			By("testing error messages and response codes")
			requests := []struct {
				path  string
				param string
				value string
				err   any
			}{
				{"/Ext/baz", "paramInt64", `true`, `failed to unmarshal request body: bad input: expecting number ; instead got true`},
				{"/Ext/baz", "paramBool", `"asdf"`, `failed to unmarshal request body: bad input: expecting boolean ; instead got asdf`},
				{"/Ext/baz", "paramString", `123`, `failed to unmarshal request body: bad input: expecting string ; instead got 123`},
				{"/Ext/baz", "paramBytes", `false`, `failed to unmarshal request body: bad input: expecting string ; instead got false`},
				{"/Ext/baz", "paramRepeatedString", `[1, 2]`, `failed to unmarshal request body: bad input: expecting string ; instead got 1`},
				{"/Ext/baz", "paramFloat64", `"a"`, `failed to unmarshal request body: strconv.ParseFloat: parsing "a": invalid syntax`},
				{"/Ext/baz", "paramEnum", `"1.5"`, `failed to unmarshal request body: enum "ext.BazRequest.BazEnum" does not have value named "1.5"`},
				{"/Ext/baz", "paramInt64", "1.5", `failed to unmarshal request body: strconv.ParseInt: parsing "1.5": invalid syntax`},
				{"/Ext/baz", "paramBool", `"true"`, nil},
				{"/Ext/baz", "paramString", `"asdf"`, nil},
				{"/Ext/baz", "paramBytes", `"asdf"`, nil},
				{"/Ext/baz", "paramRepeatedString", `["a", "b"]`, nil},
				{"/Ext/baz", "paramFloat64", "1.5", nil},
				{"/Ext/baz", "paramEnum", `"BAR"`, nil},
				{"/Ext/baz/true/asdf/BAR", "paramInt64", "1", nil},
				{"/Ext/baz/testing", "paramInt64", "1", nil},
			}
			for _, req := range requests {
				resp, err = http.Post(tv.httpEndpoint+req.path,
					"application/json", strings.NewReader(fmt.Sprintf(`{%q: %s}`, req.param, req.value)))
				Expect(err).NotTo(HaveOccurred())
				body, err = io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
				msg := string(body)
				testlog.Log.Debugf("code: %d, msg: %s", resp.StatusCode, msg)
				if req.err != nil {
					Expect(resp.StatusCode).To(Equal(400))
					Expect(msg).To(Equal(req.err))
				} else {
					Expect(resp.StatusCode).To(Equal(200))
				}
			}
			break
		}
	})
	It("should handle server streaming RPCs", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()
		client := ext.NewExtClient(cc)
		stream, err := client.ServerStream(context.Background(), &ext.FooRequest{Request: "hello"})
		Expect(err).NotTo(HaveOccurred())
		md, err := stream.Header()
		Expect(err).NotTo(HaveOccurred())
		Expect(md).To(Equal(metadata.Pairs("foo", "header", "content-type", "application/grpc")))
		for i := 0; i < 10; i++ {
			resp, err := stream.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Response).To(Equal("HELLO"))
		}
		_, err = stream.Recv()
		Expect(err).To(Equal(io.EOF))

		trailer := stream.Trailer()
		Expect(trailer).To(Equal(metadata.Pairs("foo", "trailer")))
	})
	It("should handle client streaming RPCs", func() {
		cc, err := grpc.Dial(tv.grpcEndpoint,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()
		client := ext.NewExtClient(cc)
		stream, err := client.ClientStream(context.Background())
		Expect(err).NotTo(HaveOccurred())

		md, err := stream.Header()
		Expect(err).NotTo(HaveOccurred())
		Expect(md).To(Equal(metadata.Pairs("foo", "header", "content-type", "application/grpc")))

		for i := 0; i < 5; i++ {
			err := stream.Send(&ext.FooRequest{Request: "hello"})
			Expect(err).NotTo(HaveOccurred())
		}
		resp, err := stream.CloseAndRecv()
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Response).To(Equal("hello,hello,hello,hello,hello"))

		trailer := stream.Trailer()
		Expect(trailer).To(Equal(metadata.Pairs("foo", "trailer")))
	})
	Context("error handling", func() {
		When("the plugin's Descriptor method returns an error", func() {
			BeforeEach(func() {
				descriptorLogic = func() (*apiextensions.ServiceDescriptorProtoList, error) {
					return nil, fmt.Errorf("test error")
				}
				DeferCleanup(func() {
					descriptorLogic = nil
				})
			})
			It("should not load the api extension", func() {
				extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
				Expect(err).NotTo(HaveOccurred())
				Expect(extensions.Items).To(HaveLen(0))
			})
		})
	})
	When("the plugin is not serving the service returned from Descriptor", func() {
		BeforeEach(func() {
			descriptorLogic = func() (*apiextensions.ServiceDescriptorProtoList, error) {
				return &apiextensions.ServiceDescriptorProtoList{
					Items: []*descriptorpb.ServiceDescriptorProto{
						{
							Name: lo.ToPtr("NotExt"),
						},
					},
				}, nil
			}
			DeferCleanup(func() {
				descriptorLogic = nil
			})
		})
		It("should not load the api extension", func() {
			extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(extensions.Items).To(HaveLen(0))
		})
	})
	When("the service has no http rules", func() {
		BeforeEach(func() {
			shouldLoadExt1.Store(false)
			shouldLoadExt2.Store(true)
			DeferCleanup(func() {
				shouldLoadExt1.Store(true)
				shouldLoadExt2.Store(false)
			})
		})
		It("should load the extension, but not serve its api over http", func() {
			extensions, err := tv.client.APIExtensions(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			Expect(extensions.Items).To(HaveLen(1))
			Expect(extensions.Items[0].ServiceDesc).NotTo(BeNil())
			Expect(extensions.Items[0].Rules).To(BeEmpty())
		})
	})
})

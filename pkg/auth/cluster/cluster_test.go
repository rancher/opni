package cluster_test

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func doPingPong(tc testgrpc.StreamServiceClient) error {
	stream, err := tc.Stream(context.Background())
	if err != nil {
		return err
	}
	if err := stream.Send(&testgrpc.StreamRequest{
		Request: "hello",
	}); err != nil {
		return err
	}
	reply, err := stream.Recv()
	if err != nil {
		return err
	}
	if reply.Response != "hello" {
		return fmt.Errorf("Got reply %q, want %q", reply.Response, "hello")
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != io.EOF {
		return err
	}
	return nil
}

var _ = Describe("Cluster Auth", Ordered, Label("unit"), func() {
	var ctrl *gomock.Controller
	var interceptor grpc.StreamServerInterceptor
	var server *grpc.Server
	var broker storage.KeyringStoreBroker
	testServer := &testgrpc.StreamServer{
		ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
			defer GinkgoRecover()
			Expect(cluster.StreamAuthorizedID(stream.Context())).To(Equal("foo"))
			Expect(cluster.StreamAuthorizedKeys(stream.Context())).NotTo(BeNil())
			req, err := stream.Recv()
			Expect(err).NotTo(HaveOccurred())
			Expect(req.Request).To(Equal("hello"))
			Expect(stream.Send(&testgrpc.StreamResponse{
				Response: "hello",
			})).NotTo(HaveOccurred())

			outgoingCtx := cluster.AuthorizedOutgoingContext(stream.Context())
			Expect(cluster.AuthorizedIDFromIncomingContext(outgoingCtx)).To(BeEmpty())
			Expect(cluster.AuthorizedIDFromIncomingContext(context.Background())).To(BeEmpty())
			outgoingMetadata, ok := metadata.FromOutgoingContext(outgoingCtx)
			Expect(ok).To(BeTrue())
			Expect(outgoingMetadata.Get(string(cluster.ClusterIDKey))).To(ConsistOf("foo"))
			incomingContext := metadata.NewIncomingContext(context.Background(), outgoingMetadata)
			authIncoming, ok := cluster.AuthorizedIDFromIncomingContext(incomingContext)
			Expect(ok).To(BeTrue())
			Expect(authIncoming).To(Equal("foo"))

			return nil
		},
	}
	var listener *bufconn.Listener
	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
		broker = test.NewTestKeyringStoreBroker(ctrl)
		mw, err := cluster.New(context.Background(), broker, "X-Test")
		Expect(err).NotTo(HaveOccurred())
		interceptor = mw.StreamServerInterceptor()
		server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(interceptor))
		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener = bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		DeferCleanup(listener.Close)
	})
	It("should succeed", func() {
		broker.KeyringStore("gateway", &v1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)
		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.NewClientStreamInterceptor("foo", testSharedKeys)))
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()

		client := testgrpc.NewStreamServiceClient(cc)
		Expect(doPingPong(client)).To(Succeed())
	})
	When("a keyring is not found for the agent id", func() {
		It("should fail", func() {
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.NewClientStreamInterceptor("nonexistent", testSharedKeys)))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			err = doPingPong(client)
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
	When("the agent sends the wrong nonce in its response", func() {
		It("should fail", func() {
			broker.KeyringStore("gateway", &v1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				challenge := &corev1.Challenge{}
				err = clientStream.RecvMsg(challenge)
				if err != nil {
					return nil, err
				}
				nonce := uuid.New()
				mac, err := b2mac.New512([]byte("foo"), nonce, []byte(method), testSharedKeys.ClientKey)
				if err != nil {
					return nil, err
				}
				authHeader, err := b2mac.EncodeAuthHeader([]byte("foo"), nonce, mac)
				if err != nil {
					return nil, err
				}
				err = clientStream.SendMsg(&corev1.ChallengeResponse{
					Authorization: authHeader,
				})
				if err != nil {
					return nil, err
				}

				return clientStream, err
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			err = doPingPong(client)
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
	When("the agent sends the wrong mac in its response", func() {
		It("should fail", func() {
			broker.KeyringStore("gateway", &v1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				challenge := &corev1.Challenge{}
				err = clientStream.RecvMsg(challenge)
				if err != nil {
					return nil, err
				}
				nonce := util.Must(uuid.FromBytes(challenge.Nonce))
				mac, err := b2mac.New512([]byte("foo"), nonce, []byte(method), []byte("wrong"))
				if err != nil {
					return nil, err
				}
				authHeader, err := b2mac.EncodeAuthHeader([]byte("foo"), nonce, mac)
				if err != nil {
					return nil, err
				}
				err = clientStream.SendMsg(&corev1.ChallengeResponse{
					Authorization: authHeader,
				})
				if err != nil {
					return nil, err
				}

				return clientStream, err
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			err = doPingPong(client)
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
})

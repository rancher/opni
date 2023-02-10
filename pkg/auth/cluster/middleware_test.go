package cluster_test

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util/streams"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type (
	contextKey string
)

const (
	contextValue contextKey = "value"
)

var (
	errPrimaryChallengeFailed = status.Errorf(codes.Unauthenticated, "primary challenge failed")
)

var _ = Describe("Cluster Auth Middleware", Ordered, Label("unit"), func() {
	var listener *bufconn.Listener
	var testServer *testgrpc.StreamServer
	var testClient testgrpc.StreamServiceClient
	var clientId string
	var expectedMetadata []string
	serverChallengeHandler := test.NewTestChallengeHandler(
		func(ss streams.Stream) (context.Context, error) {
			md, ok := metadata.FromIncomingContext(ss.Context())
			if !ok {
				return nil, errors.New("fail: no metadata")
			}
			if !slices.Equal(md.Get("md-key"), expectedMetadata) {
				return nil, errors.New("fail: invalid metadata")
			}

			var msg testgrpc.StreamRequest
			err := ss.RecvMsg(&msg)
			if err != nil {
				return nil, err
			}
			if msg.Request == "fail-challenge" {
				return nil, errPrimaryChallengeFailed
			}
			if err := ss.SendMsg(&testgrpc.StreamResponse{Response: "value:" + msg.Request}); err != nil {
				return nil, err
			}

			return context.WithValue(ss.Context(), contextValue, msg.Request), nil
		})
	clientChallengeHandler := test.NewTestChallengeHandler(
		func(cs streams.Stream) (context.Context, error) {
			md, ok := metadata.FromOutgoingContext(cs.Context())
			if !ok {
				return nil, errors.New("fail: no metadata")
			}
			if !slices.Equal(md.Get("md-key"), expectedMetadata) {
				return nil, errors.New("fail: invalid metadata")
			}

			if err := cs.SendMsg(&testgrpc.StreamRequest{Request: clientId}); err != nil {
				return nil, err
			}
			var msg testgrpc.StreamResponse
			if err := cs.RecvMsg(&msg); err != nil {
				return nil, err
			}
			if msg.Response != "value:"+clientId {
				return nil, errors.New("(test) invalid response: " + msg.Response)
			}
			return context.WithValue(cs.Context(), contextValue, clientId), nil
		},
		"md-key", "value",
	)
	serverHandler := func(stream testgrpc.StreamService_StreamServer) error {
		return nil
	}

	BeforeAll(func() {
		testServer = &testgrpc.StreamServer{
			ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
				return serverHandler(stream)
			},
		}
	})

	JustBeforeEach(func() {
		server := grpc.NewServer(
			grpc.Creds(insecure.NewCredentials()),
			grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverChallengeHandler)),
		)
		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener = bufconn.Listen(1024 * 1024)
		go server.Serve(listener)
		DeferCleanup(listener.Close)

		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientChallengeHandler)))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cc.Close)

		testClient = testgrpc.NewStreamServiceClient(cc)
	})
	When("creating new server middleware", func() {
		When("a challenge is configured", func() {
			id := uuid.NewString()
			BeforeEach(func() {
				clientId = id
				expectedMetadata = []string{"value"}
				serverHandler = func(stream testgrpc.StreamService_StreamServer) error {
					Expect(stream.Context().Value(contextValue)).To(Equal(id))
					return nil
				}
			})
			It("should run the challenge", func() {
				client, err := testClient.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				_, err = client.Recv()
				Expect(err).To(Equal(io.EOF))
			})
		})
	})
	When("the challenge fails", func() {
		BeforeEach(func() {
			clientId = "fail-challenge"
			expectedMetadata = []string{"value"}
			serverHandler = func(stream testgrpc.StreamService_StreamServer) error {
				panic("server handler should not be called")
			}
		})
		It("should return an error", func() {
			_, err := testClient.Stream(context.Background())
			Expect(err).To(MatchError(errPrimaryChallengeFailed))
		})
	})
	When("a client interceptor fails", func() {
		It("should return an error", func() {
			errTest := errors.New("client interceptor failed")
			cs, err := cluster.StreamClientInterceptor(clientChallengeHandler)(context.Background(), nil, nil, "", grpc.Streamer(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				return nil, errTest
			}))
			Expect(err).To(MatchError(errTest))
			Expect(cs).To(BeNil())
		})
	})
})

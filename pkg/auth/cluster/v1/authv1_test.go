package authv1_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv1 "github.com/rancher/opni/pkg/auth/cluster/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Cluster Auth V1 Compatibility", Ordered, Label("unit"), func() {
	var server *grpc.Server
	var broker storage.KeyringStoreBroker
	clusterId := uuid.NewString()
	testServer := &testgrpc.StreamServer{
		ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
			Expect(cluster.StreamAuthorizedID(stream.Context())).To(Equal(clusterId))
			Expect(cluster.StreamAuthorizedKeys(stream.Context())).NotTo(BeNil())

			return nil
		},
	}
	var listener *bufconn.Listener
	var serverMw challenges.ChallengeHandler
	BeforeAll(func() {
		broker = test.NewTestKeyringStoreBroker(ctrl)
		verifier := challenges.NewKeyringVerifier(broker, test.Log)
		serverMw = authv1.NewServerChallenge("/testgrpc.stream.StreamService/Stream", verifier, test.Log)
		server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverMw)))
		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener = bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		DeferCleanup(listener.Close)
	})
	When("a v1 agent connects to the legacy service", func() {
		It("should succeed", func() {
			broker.KeyringStore("gateway", &corev1.Reference{Id: clusterId}).Put(context.Background(), testKeyring)
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				headerMd, err := clientStream.Header()
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to read stream header: %v", err)
				}
				challenge := headerMd.Get(challenges.ChallengeKey)
				if len(challenge) != 1 {
					return nil, status.Errorf(codes.Internal, "server did not send a challenge header")
				}
				nonce, err := uuid.Parse(challenge[0])
				if err != nil {
					return nil, status.Errorf(codes.Internal, "server sent an invalid challenge header")
				}
				mac, err := newMac(clusterId, nonce, method, testSharedKeys.ClientKey)
				if err != nil {
					return nil, err
				}
				authHeader := encodeAuthHeader([]byte(clusterId), nonce, mac)

				err = clientStream.SendMsg(&corev1.ChallengeResponse{
					Response: []byte(authHeader),
				})
				if err != nil {
					return nil, err
				}

				err = clientStream.RecvMsg(nil)

				return clientStream, err
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(err).To(MatchError(io.EOF))
		})
	})
	When("the agent replies with the wrong nonce", func() {
		It("should fail", func() {
			broker.KeyringStore("gateway", &corev1.Reference{Id: clusterId}).Put(context.Background(), testKeyring)
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				headerMd, err := clientStream.Header()
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to read stream header: %v", err)
				}
				challenge := headerMd.Get(challenges.ChallengeKey)
				if len(challenge) != 1 {
					return nil, status.Errorf(codes.Internal, "server did not send a challenge header")
				}
				nonce, err := uuid.Parse(challenge[0])
				if err != nil {
					return nil, status.Errorf(codes.Internal, "server sent an invalid challenge header")
				}
				mac, err := newMac(clusterId, nonce, method, testSharedKeys.ClientKey)
				if err != nil {
					return nil, err
				}
				authHeader := encodeAuthHeader([]byte(clusterId), uuid.New(), mac)

				err = clientStream.SendMsg(&corev1.ChallengeResponse{
					Response: []byte(authHeader),
				})
				if err != nil {
					return nil, err
				}

				return nil, clientStream.RecvMsg(nil)
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
	When("the agent sends the wrong mac in its response", func() {
		It("should fail", func() {
			broker.KeyringStore("gateway", &corev1.Reference{Id: clusterId}).Put(context.Background(), testKeyring)
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				headerMd, err := clientStream.Header()
				if err != nil {
					return nil, status.Errorf(codes.Internal, "failed to read stream header: %v", err)
				}
				challenge := headerMd.Get(challenges.ChallengeKey)
				if len(challenge) != 1 {
					return nil, status.Errorf(codes.Internal, "server did not send a challenge header")
				}
				nonce, err := uuid.Parse(challenge[0])
				if err != nil {
					return nil, status.Errorf(codes.Internal, "server sent an invalid challenge header")
				}
				mac, err := newMac(clusterId, nonce, method, []byte("wrong"))
				if err != nil {
					return nil, err
				}
				authHeader := encodeAuthHeader([]byte(clusterId), nonce, mac)

				err = clientStream.SendMsg(&corev1.ChallengeResponse{
					Response: []byte(authHeader),
				})
				if err != nil {
					return nil, err
				}

				return nil, clientStream.RecvMsg(nil)
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
})

func newMac(id string, nonce uuid.UUID, method string, key []byte) ([]byte, error) {
	mac, _ := blake2b.New512(key)
	mac.Write([]byte(id))
	mac.Write(nonce[:])
	mac.Write([]byte(method))
	return mac.Sum(nil), nil
}

func encodeAuthHeader(id []byte, nonce uuid.UUID, mac []byte) string {
	idEncoded := base64.RawURLEncoding.EncodeToString(id)
	nonceEncoded := nonce.String()
	macEncoded := base64.RawURLEncoding.EncodeToString(mac)
	return fmt.Sprintf(`MAC id="%s",nonce="%s",mac="%s"`, idEncoded, nonceEncoded, macEncoded)
}

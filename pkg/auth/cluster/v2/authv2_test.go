package authv2_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	mock_auth "github.com/rancher/opni/pkg/test/mock/auth"
	mock_grpc "github.com/rancher/opni/pkg/test/mock/grpc"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	mock_streams "github.com/rancher/opni/pkg/test/mock/streams"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/streams"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Cluster Auth V2", Ordered, Label("unit"), func() {
	var server *grpc.Server
	var broker storage.KeyringStoreBroker

	testServer := &testgrpc.StreamServer{
		ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
			return nil
		},
	}

	outgoingCtx := func(ctx context.Context) context.Context {
		md := metadata.Pairs(
			challenges.ClientIdAssertionMetadataKey, "foo",
			challenges.ClientRandomMetadataKey, "bm90aGluZy11cC1teS1zbGVldmU",
			challenges.ChallengeVersionMetadataKey, challenges.ChallengeV2,
		)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return ctx
	}

	addKeyring := func() {
		broker.KeyringStore("gateway", &corev1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)
	}
	var listener *bufconn.Listener
	var serverMw challenges.ChallengeHandler
	var clientMw challenges.ChallengeHandler
	var verifier challenges.KeyringVerifier
	var serverInterceptors []grpc.StreamServerInterceptor
	JustBeforeEach(func() {
		broker = mock_storage.NewTestKeyringStoreBroker(ctrl)
		var err error
		verifier = challenges.NewKeyringVerifier(broker, authv2.DomainString, testlog.Log)
		Expect(err).NotTo(HaveOccurred())
		clientMw, err = authv2.NewClientChallenge(testKeyring, "foo", testlog.Log)
		Expect(err).NotTo(HaveOccurred())
		serverMw = authv2.NewServerChallenge(verifier, testlog.Log, authv2.WithChallengeTimeout(100*time.Millisecond))

		server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()),
			grpc.ChainStreamInterceptor(append(serverInterceptors, cluster.StreamServerInterceptor(serverMw))...))

		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener = bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		DeferCleanup(listener.Close)
	})
	It("should succeed", func() {
		addKeyring()
		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
		Expect(err).NotTo(HaveOccurred())
		defer cc.Close()

		client := testgrpc.NewStreamServiceClient(cc)
		_, err = client.Stream(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})
	When("a keyring is not found for the agent id", func() {
		It("should fail", func() {
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
	When("the agent does not send the correct request metadata", func() {
		It("should fail", func() {
			addKeyring()
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				stream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return nil, err
				}
				var req corev1.ChallengeRequestList
				if err := stream.RecvMsg(&req); err != nil {
					return nil, err
				}
				Fail("did not receive expected error")
				return stream, nil
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument))
		})
	})

	When("the agent sends the wrong mac in its response", func() {
		It("should fail", func() {
			addKeyring()
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				ctx = outgoingCtx(ctx)
				clientStream, err := streamer(ctx, desc, cc, method, opts...)
				if err != nil {
					return clientStream, err
				}
				var req corev1.ChallengeRequestList
				if err := clientStream.RecvMsg(&req); err != nil {
					return nil, err
				}

				err = clientStream.SendMsg(&corev1.ChallengeResponseList{
					Items: []*corev1.ChallengeResponse{
						{
							Response: []byte("wrong"),
						},
					},
				})
				if err != nil {
					return nil, err
				}

				var authInfo corev1.AuthInfo
				err = clientStream.RecvMsg(&authInfo)
				if err != nil {
					return nil, err
				}

				return clientStream, err
			}))
			Expect(err).NotTo(HaveOccurred())
			defer cc.Close()

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
		})
	})
	When("the agent uses an incorrect key to sign its response", func() {
		It("should fail", func() {
			addKeyring()
			invalidKeyring := keyring.New(keyring.NewSharedKeys(make([]byte, 64)), sessionAttrKey1, sessionAttrKey2)

			clientMw, err := authv2.NewClientChallenge(invalidKeyring, "foo", testlog.Log)
			Expect(err).NotTo(HaveOccurred())

			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
			Expect(err).NotTo(HaveOccurred())

			client := testgrpc.NewStreamServiceClient(cc)
			_, err = client.Stream(context.Background())
			Expect(status.Code(err)).To(Equal(codes.Unauthenticated))

		})
	})

	Context("error handling", func() {
		When("creating a new v2 client and the keyring is missing shared keys", func() {
			It("should return a DataLoss error", func() {
				_, err := authv2.NewClientChallenge(keyring.New(), "foo", testlog.Log)
				Expect(status.Code(err)).To(Equal(codes.DataLoss))
			})
		})
		When("an error occurs sending a challenge response", func() {
			It("should return an error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					streamer(ctx, desc, cc, method, opts...)
					return nil, errors.New("test error")
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(err).To(HaveOccurred())
			})
		})
		When("waiting for a challenge response times out", func() {
			It("should return a DeadlineExceeded error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					clientStream, err := streamer(ctx, desc, cc, method, opts...)
					if err != nil {
						return clientStream, err
					}
					var req corev1.ChallengeRequestList
					err = clientStream.RecvMsg(&req)
					if err != nil {
						return nil, err
					}
					time.Sleep(110 * time.Millisecond) // wait for the deadline to expire
					// the server should have timed out waiting for the challenge response
					// and closed the stream
					var x corev1.ChallengeResponseList
					err = clientStream.SendMsg(&x)
					if !errors.Is(err, io.EOF) {
						panic("test failed")
					}
					err = clientStream.RecvMsg(nil)
					if err == nil {
						panic("test failed")
					}
					return nil, err
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(status.Code(err)).To(Equal(codes.DeadlineExceeded), err.Error())
				Expect(err.Error()).To(ContainSubstring("timed out waiting for challenge response"))
			})
		})
		When("the stream context is canceled while waiting for a challenge response", func() {
			It("should return a Cancelled error", func() {
				addKeyring()
				acquired := make(chan struct{})
				cancelled := make(chan struct{})
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					defer close(cancelled)
					ctx = outgoingCtx(ctx)
					clientStream, err := streamer(ctx, desc, cc, method, opts...)
					if err != nil {
						if errors.Is(err, ctx.Err()) && strings.Contains(err.Error(), "context deadline exceeded") {
							// process/goroutine hang, retry test
							return nil, err
						}
						panic("test failed: unexpected error: " + err.Error())
					}
					var req corev1.ChallengeRequestList
					clientStream.RecvMsg(&req)
					acquired <- struct{}{}
					<-cancelled
					time.Sleep(15 * time.Millisecond)
					// at this point, ctx should have been canceled by us, so sending a
					// challenge response should fail with a DeadlineExceeded error
					var x corev1.ChallengeResponseList
					err = clientStream.SendMsg(&x)
					if !errors.Is(err, io.EOF) {
						panic("test failed")
					}
					err = clientStream.RecvMsg(nil)
					if err == nil {
						panic("test failed")
					}
					return nil, err
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				// cancel the context before the server timeout expires
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go func() {
					defer close(acquired)
					<-acquired
					cancel()
					cancelled <- struct{}{}
				}()

				_, err = client.Stream(ctx)

				Expect(status.Code(err)).To(Equal(codes.Canceled))
				Expect(err.Error()).To(ContainSubstring("context canceled"))
			})
		})
		When("an error occurs reading the challenge response", func() {
			It("should return an Aborted error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					clientStream, _ := streamer(ctx, desc, cc, method, opts...)
					var req corev1.ChallengeRequestList
					clientStream.RecvMsg(&req)
					clientStream.CloseSend() // close the stream to cause an error

					// try to get the stream status
					err := clientStream.RecvMsg(nil)
					if err == nil {
						panic("test failed")
					}
					return nil, err
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(status.Code(err)).To(Equal(codes.Aborted))
			})
		})
		When("a received message doesn't look like a challenge response", func() {
			It("should return an InvalidArgument error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					clientStream, _ := streamer(ctx, desc, cc, method, opts...)
					var req corev1.ChallengeRequestList
					clientStream.RecvMsg(&req)
					err := clientStream.SendMsg(&corev1.ClusterList{Items: []*corev1.Cluster{{Id: "foo"}}})
					if err != nil {
						return nil, err
					}
					// try to get the stream status
					err = clientStream.RecvMsg(nil)
					if err == nil {
						panic("test failed")
					}

					return nil, err
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(err.Error()).To(ContainSubstring("expected challenge response, but received incorrect message data"))
			})
		})
		When("an empty challenge response is received", func() {
			It("should return an InvalidArgument error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					clientStream, _ := streamer(ctx, desc, cc, method, opts...)
					var req corev1.ChallengeRequestList
					clientStream.RecvMsg(&req)
					err := clientStream.SendMsg(&corev1.ChallengeResponseList{})
					if err != nil {
						return nil, err
					}
					// try to get the stream status
					err = clientStream.RecvMsg(nil)
					if err == nil {
						panic("test failed")
					}
					return nil, err
				}))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(err.Error()).To(ContainSubstring("invalid challenge response received"))
			})
		})
		When("an error occurs receiving the auth info message", func() {
			var testError = errors.New("test error")
			BeforeEach(func() {
				serverInterceptors = []grpc.StreamServerInterceptor{
					func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						m := mock_grpc.NewMockServerStream(ctrl)
						m.EXPECT().SendHeader(gomock.Any()).DoAndReturn(ss.SendHeader).AnyTimes()
						m.EXPECT().SetHeader(gomock.Any()).DoAndReturn(ss.SetHeader).AnyTimes()
						m.EXPECT().SetTrailer(gomock.Any()).DoAndReturn(ss.SetTrailer).AnyTimes()
						m.EXPECT().Context().DoAndReturn(ss.Context).AnyTimes()
						m.EXPECT().RecvMsg(gomock.Any()).DoAndReturn(ss.RecvMsg).AnyTimes()
						gomock.InOrder(
							m.EXPECT().
								SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.AuthInfo{}))).
								DoAndReturn(ss.SendMsg).
								AnyTimes(),
							m.EXPECT().
								SendMsg(gomock.AssignableToTypeOf(&corev1.AuthInfo{})).
								Return(testError).
								Times(1),
						)

						return handler(srv, m)
					},
				}
				DeferCleanup(func() {
					serverInterceptors = nil
				})
			})
			It("should return an error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
				Expect(err).NotTo(HaveOccurred())

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, ContainSubstring("test error")))
			})
		})

		When("there is no outgoing metadata in the client handler", func() {
			It("should return an InvalidArgument error", func() {
				s := mock_streams.NewMockStream(ctrl)
				s.EXPECT().Context().Return(context.Background()).Times(1)
				ctx, err := clientMw.DoChallenge(s)
				Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, ContainSubstring("missing metadata")))
				Expect(ctx).To(BeNil())
			})
		})

		When("an error occurs sending the challenge response", func() {
			It("should return an error", func() {
				clientChallenge, err := authv2.NewClientChallenge(testKeyring, "foo", testlog.Log)
				Expect(err).NotTo(HaveOccurred())

				ctx := outgoingCtx(context.Background())
				mockClientStream := mock_grpc.NewMockClientStream(ctrl)
				mockClientStream.EXPECT().Context().Return(ctx).AnyTimes()
				recv := mockClientStream.EXPECT().RecvMsg(gomock.Any()).DoAndReturn(func(msg interface{}) error {
					m := msg.(*corev1.ChallengeRequestList)
					m.Items = []*corev1.ChallengeRequest{
						{Challenge: make([]byte, 32)},
					}
					return nil
				}).Times(1)
				mockClientStream.EXPECT().
					SendMsg(gomock.Any()).
					After(recv).
					Return(errors.New("send error")).
					Times(1)

				_, err = clientChallenge.DoChallenge(mockClientStream)
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, ContainSubstring("error sending challenge response")))
			})
		})

		When("the server sends a challenge request with the wrong number of challenges", func() {
			It("should return an error", func() {
				handler := mock_auth.NewTestChallengeHandler(func(ss streams.Stream) (context.Context, error) {
					challenge := authutil.NewRandom256()
					req := corev1.ChallengeRequestList{
						Items: []*corev1.ChallengeRequest{
							{Challenge: challenge[:]},
							{Challenge: challenge[:]},
						},
					}
					if err := ss.SendMsg(&req); err != nil {
						return nil, err
					}
					return nil, nil
				})
				server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(cluster.StreamServerInterceptor(handler)))
				testgrpc.RegisterStreamServiceServer(server, testServer)
				listener = bufconn.Listen(1024 * 1024)
				go server.Serve(listener)
				DeferCleanup(listener.Close)

				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
				Expect(err).NotTo(HaveOccurred())
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, ContainSubstring("invalid challenge request received from server")))
			})
		})

		When("an error occurs receiving the auth info message", func() {
			It("should return an error", func() {
				stop := make(chan struct{})
				handler := mock_auth.NewTestChallengeHandler(func(ss streams.Stream) (context.Context, error) {
					// send challenge request
					challenge := authutil.NewRandom256()

					req := corev1.ChallengeRequestList{
						Items: []*corev1.ChallengeRequest{
							{
								Challenge: challenge[:],
							},
						},
					}
					if err := ss.SendMsg(&req); err != nil {
						return nil, err
					}

					// receive challenge response
					var resp corev1.ChallengeResponseList
					if err := ss.RecvMsg(&resp); err != nil {
						return nil, err
					}

					// exit, causing the client to fail to receive the auth info
					stop <- struct{}{}
					<-stop

					return nil, io.EOF
				})
				var err error
				serverMw := handler
				clientMw, err = authv2.NewClientChallenge(testKeyring, "foo", testlog.Log)
				Expect(err).NotTo(HaveOccurred())
				addKeyring()
				server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverMw)))
				testgrpc.RegisterStreamServiceServer(server, testServer)
				listener = bufconn.Listen(1024 * 1024)
				go server.Serve(listener)
				go func() {
					<-stop
					listener.Close()
					close(stop)
				}()

				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
				Expect(err).NotTo(HaveOccurred())
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(status.Code(err)).To(Equal(codes.Aborted))
				Expect(err.Error()).To(ContainSubstring("error receiving auth info"))
			})
		})
		When("the auth info message does not validate", func() {
			It("should return an aborted error", func() {
				handler := mock_auth.NewTestChallengeHandler(func(ss streams.Stream) (context.Context, error) {
					// send challenge request
					challenge := authutil.NewRandom256()

					cm, _ := challenges.ClientMetadataFromIncomingContext(ss.Context())

					req := corev1.ChallengeRequestList{
						Items: []*corev1.ChallengeRequest{
							{
								Challenge: challenge[:],
							},
						},
					}
					if err := ss.SendMsg(&req); err != nil {
						return nil, err
					}

					// receive challenge response
					var resp corev1.ChallengeResponseList
					if err := ss.RecvMsg(&resp); err != nil {
						return nil, err
					}
					// send an invalid auth info message
					authInfo := corev1.AuthInfo{
						AuthorizedId: "foo",
					}

					mac, _ := blake2b.New512(make([]byte, 32))
					mac.Write([]byte(cm.IdAssertion))
					mac.Write(authutil.NewRandom256()) // replace the client random
					for _, cr := range req.Items {
						mac.Write(cr.Challenge)
					}
					for _, cr := range resp.Items {
						mac.Write(cr.Response)
					}
					mac.Write([]byte(authInfo.AuthorizedId))
					expectedMac := mac.Sum(nil)
					authInfo.Mac = expectedMac

					if err := ss.SendMsg(&authInfo); err != nil {
						return nil, err
					}

					return ss.Context(), nil
				})
				var err error
				serverMw := handler
				clientMw, err = authv2.NewClientChallenge(testKeyring, "foo", testlog.Log)
				Expect(err).NotTo(HaveOccurred())
				addKeyring()
				server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverMw)))
				testgrpc.RegisterStreamServiceServer(server, testServer)
				listener = bufconn.Listen(1024 * 1024)
				go server.Serve(listener)

				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
				Expect(err).NotTo(HaveOccurred())
				client := testgrpc.NewStreamServiceClient(cc)

				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, Equal("session info verification failed")))
			})
		})
	})
	Context("ShouldEnableIncoming", func() {
		When("incoming metadata contains the v2 challenge version", func() {
			It("should enable the challenge", func() {
				ok, err := authv2.ShouldEnableIncoming(metadata.NewIncomingContext(context.Background(), metadata.Pairs(
					challenges.ChallengeVersionMetadataKey, challenges.ChallengeV2,
				)))
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeTrue())
			})
		})
		When("incoming metadata does not contain the v2 challenge version", func() {
			It("should not enable the challenge", func() {
				ok, err := authv2.ShouldEnableIncoming(metadata.NewIncomingContext(context.Background(), metadata.Pairs(
					"foo", "bar",
				)))
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
		When("incoming metadata is nil", func() {
			It("should not enable the challenge", func() {
				ok, err := authv2.ShouldEnableIncoming(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(BeFalse())
			})
		})
		When("incoming metadata contains an unknown challenge version", func() {
			It("should return an error", func() {
				for _, v := range []string{
					"v1", "v3", "", "v2.0", "1", "2",
				} {
					_, err := authv2.ShouldEnableIncoming(metadata.NewIncomingContext(context.Background(), metadata.Pairs(
						challenges.ChallengeVersionMetadataKey, v,
					)))
					Expect(err).To(HaveOccurred())
				}
			})
		})

	})
})

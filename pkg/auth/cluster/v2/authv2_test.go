package authv2_test

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	authutil "github.com/rancher/opni/pkg/auth/util"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util/streams"
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
			Expect(cluster.StreamAuthorizedID(stream.Context())).To(Equal("foo"))
			Expect(cluster.StreamAuthorizedKeys(stream.Context())).NotTo(BeNil())
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
	BeforeEach(func() {
		broker = test.NewTestKeyringStoreBroker(ctrl)
		var err error
		verifier = challenges.NewKeyringVerifier(broker, test.Log)
		Expect(err).NotTo(HaveOccurred())
		clientMw, err = authv2.NewClientChallenge(testKeyring, "foo", test.Log)
		serverMw = authv2.NewServerChallenge(verifier, test.Log)
		server = grpc.NewServer(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(cluster.StreamServerInterceptor(serverMw)))
		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener = bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		DeferCleanup(listener.Close)
	})
	It("should succeed", func() {
		addKeyring()
		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
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
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
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
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})
	})

	When("the agent sends the wrong mac in its response", func() {
		It("should fail", func() {
			addKeyring()
			cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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

	Context("error handling", func() {
		When("creating a new v2 client and the keyring is missing shared keys", func() {
			It("should return a DataLoss error", func() {
				_, err := authv2.NewClientChallenge(keyring.New(), "foo", test.Log)
				Expect(status.Code(err)).To(Equal(codes.DataLoss))
			})
		})
		When("an error occurs sending a challenge response", func() {
			It("should return an error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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
					time.Sleep(1100 * time.Millisecond) // wait for the deadline to expire
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
			It("should return a DeadlineExceeded error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
					ctx = outgoingCtx(ctx)
					clientStream, _ := streamer(ctx, desc, cc, method, opts...)
					var req corev1.ChallengeRequestList
					clientStream.RecvMsg(&req)
					time.Sleep(500 * time.Millisecond)
					// at this point, ctx should have been canceled by us, so sending a
					// challenge response should fail with a DeadlineExceeded error
					var x corev1.ChallengeResponseList
					err := clientStream.SendMsg(&x)
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
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				_, err = client.Stream(ctx)

				Expect(status.Code(err)).To(Equal(codes.DeadlineExceeded))
				Expect(err.Error()).To(ContainSubstring("context deadline exceeded"))
			})
		})
		When("an error occurs reading the challenge response", func() {
			It("should return an Aborted error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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

				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				_, err = client.Stream(ctx)
				Expect(status.Code(err)).To(Equal(codes.Aborted))
			})
		})
		When("a received message doesn't look like a challenge response", func() {
			It("should return an InvalidArgument error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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

				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				_, err = client.Stream(ctx)
				Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
				Expect(err.Error()).To(ContainSubstring("expected challenge response, but received incorrect message data"))
			})
		})
		When("an empty challenge response is received", func() {
			It("should return an InvalidArgument error", func() {
				addKeyring()
				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}), grpc.WithInsecure(), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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

		Context("misc error scenarios", func() {
			// When("an error occurs when receiving a challenge request from the server", func() {
			// 	It("should receive the error", func() {
			// 		handler := test.NewTestChallengeHandler(
			// 			func(ss grpc.ServerStream) (context.Context, error) {
			// 				return nil, errors.New("test error")
			// 			},
			// 			func(cs grpc.ClientStream) (context.Context, error) {
			// 				return cs.Context(), nil
			// 			},
			// 			nil,
			// 		)
			// 		serverMw := challenges.NewServerMiddleware(challenges.ChallengeConfig{
			// 			Primary: handler,
			// 		})
			// 		clientMw = challenges.NewClientMiddleware(challenges.ChallengeConfig{
			// 			Primary: testutil.Must(challenges.NewClientChallenge(testKeyring, "foo", test.Log)),
			// 		})
			// 		server = grpc.NewServerChallenge(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(serverMw.StreamServerInterceptor()))
			// 		testgrpc.RegisterStreamServiceServer(server, testServer)
			// 		listener = bufconn.Listen(1024 * 1024)
			// 		go server.Serve(listener)
			// 		DeferCleanup(listener.Close)

			// 		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			// 			return listener.Dial()
			// 		}), grpc.WithInsecure(), grpc.WithStreamInterceptor(clientMw.StreamClientInterceptor()))
			// 		Expect(err).NotTo(HaveOccurred())
			// 		client := testgrpc.NewStreamServiceClient(cc)

			// 		_, err = client.Stream(context.Background())
			// 		Expect(status.Code(err)).To(Equal(codes.Aborted))
			// 		Expect(err.Error()).To(ContainSubstring("error receiving challenge request"))
			// 	})
			// })
			// When("the client fails to generate a challenge response", func() {
			// 	It("should return an internal error", func() {
			// 		var testBytes [256]byte
			// 		rand.Read(testBytes[:])

			// 		serverMw := challenges.NewServerMiddleware(challenges.ChallengeConfig{
			// 			Primary: testutil.Must(challenges.NewServerChallenge(verifier, test.Log)),
			// 		})
			// 		clientMw = challenges.NewClientMiddleware(challenges.ChallengeConfig{
			// 			Primary: testutil.Must(challenges.NewClientChallenge(keyring.New(&keyring.SharedKeys{
			// 				ClientKey: testBytes[:128], // key too large
			// 				ServerKey: testBytes[128:],
			// 			}), "id", test.Log)),
			// 		})
			// 		server = grpc.NewServerChallenge(grpc.Creds(insecure.NewCredentials()), grpc.StreamInterceptor(serverMw.StreamServerInterceptor()))
			// 		testgrpc.RegisterStreamServiceServer(server, testServer)
			// 		listener = bufconn.Listen(1024 * 1024)
			// 		go server.Serve(listener)
			// 		DeferCleanup(listener.Close)

			// 		cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			// 			return listener.Dial()
			// 		}), grpc.WithInsecure(), grpc.WithStreamInterceptor(clientMw.StreamClientInterceptor()))
			// 		Expect(err).NotTo(HaveOccurred())
			// 		client := testgrpc.NewStreamServiceClient(cc)

			// 		_, err = client.Stream(context.Background())
			// 		Expect(status.Code(err)).To(Equal(codes.Internal))
			// 		Expect(err.Error()).To(ContainSubstring("failed to generate challenge response"))
			// 	})
			// })

			When("an error occurs sending the challenge response", func() {
				It("should return an error", func() {
					stop := make(chan struct{})
					handler := test.NewTestChallengeHandler(func(ss streams.Stream) (context.Context, error) {
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

						// exit, causing the client to fail to send the response
						stop <- struct{}{}
						<-stop

						return nil, io.EOF
					})
					var err error
					serverMw := handler
					clientMw, err = authv2.NewClientChallenge(testKeyring, "foo", test.Log)
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
					}), grpc.WithInsecure(), grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(clientMw)))
					Expect(err).NotTo(HaveOccurred())
					client := testgrpc.NewStreamServiceClient(cc)

					_, err = client.Stream(context.Background())
					Expect(status.Code(err)).To(Equal(codes.Aborted))
					Expect(err.Error()).To(ContainSubstring("error sending challenge response"))
				})
			})
		})
	})
})

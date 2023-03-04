package session_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/test"
	mock_grpc "github.com/rancher/opni/pkg/test/mock/grpc"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/streams"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

var _ = Describe("Session Attributes Challenge", Ordered, Label("unit"), func() {
	var (
		goodKey1, goodKey2,
		badKey1, badKey2,
		skipKey1, skipKey2 *ephemeral.Key
	)
	serverHandler := &atomic.Pointer[func(stream testgrpc.StreamService_StreamServer) error]{}
	var dialer func(context.Context, string) (net.Conn, error)
	var dial = func(kr ...keyring.Keyring) (*grpc.ClientConn, error) {
		var clientKeyring keyring.Keyring
		if len(kr) > 0 {
			clientKeyring = kr[0]
		} else {
			clientKeyring = testKeyring
		}
		clientChallenge, err := session.NewClientChallenge(clientKeyring)
		Expect(err).NotTo(HaveOccurred())

		handler := challenges.Chained(
			testutil.Must(authv2.NewClientChallenge(clientKeyring, "foo", test.Log)),
			challenges.If(clientChallenge.HasAttributes).Then(clientChallenge),
		)

		return grpc.Dial("bufconn", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithStreamInterceptor(cluster.StreamClientInterceptor(handler)))
	}
	var serverInterceptors []grpc.StreamServerInterceptor

	goodKey1 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeLabelKey: "good1",
	})
	goodKey2 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeLabelKey: "good2",
	})
	badKey1 = &ephemeral.Key{
		Usage:  ephemeral.Authentication,
		Secret: make([]byte, 33),
		Labels: map[string]string{
			session.AttributeLabelKey: "bad1",
		},
	}
	badKey2 = &ephemeral.Key{
		Usage:  "foo",
		Secret: make([]byte, 32),
		Labels: map[string]string{
			session.AttributeLabelKey: "bad2",
		},
	}
	skipKey1 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		"foo": "skip1", // wrong key
	})
	skipKey2 = ephemeral.NewKey(ephemeral.Authentication, map[string]string{
		session.AttributeMetadataKey: "skip2", // note the wrong constant
	})
	serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
		return nil
	}))
	testServer := &testgrpc.StreamServer{
		ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
			return (*serverHandler.Load())(stream)
		},
	}

	JustBeforeEach(func() {
		attrChallenge, err := session.NewServerChallenge(testKeyring, session.WithAttributeRequestLimit(50))
		Expect(err).NotTo(HaveOccurred())

		broker := test.NewTestKeyringStoreBroker(ctrl)
		broker.KeyringStore("gateway", &corev1.Reference{Id: "foo"}).Put(context.Background(), testKeyring)

		verifier := challenges.NewKeyringVerifier(broker, authv2.DomainString, test.Log)

		serverMw := challenges.Chained(
			testutil.Must(authv2.NewServerChallenge(verifier, test.Log)),
			challenges.If(session.ShouldEnableIncoming).Then(attrChallenge),
		)

		server := grpc.NewServer(
			grpc.MaxHeaderListSize(1024*8),
			grpc.Creds(insecure.NewCredentials()),
			grpc.ChainStreamInterceptor(append(serverInterceptors, cluster.StreamServerInterceptor(serverMw))...))

		testgrpc.RegisterStreamServiceServer(server, testServer)
		listener := bufconn.Listen(1024 * 1024)
		go server.Serve(listener)

		dialer = func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}

		DeferCleanup(listener.Close)
	})
	When("creating a new challenge handler", func() {
		When("there are no ephemeral keys in the keyring", func() {
			It("should succeed, but have no attributes", func() {
				s, err := session.NewServerChallenge(keyring.New())
				Expect(err).NotTo(HaveOccurred())
				Expect(s).NotTo(BeNil())
				Expect(s.Attributes()).To(BeEmpty())

				c, err := session.NewClientChallenge(keyring.New())
				Expect(err).NotTo(HaveOccurred())
				Expect(c).NotTo(BeNil())
				Expect(c.Attributes()).To(BeEmpty())
			})
			Specify("InterceptContext should be a no-op", func() {
				s, err := session.NewServerChallenge(keyring.New())
				Expect(err).NotTo(HaveOccurred())
				Expect(s.InterceptContext(context.Background())).To(Equal(context.Background()))

				c, err := session.NewClientChallenge(keyring.New())
				Expect(err).NotTo(HaveOccurred())
				Expect(c.InterceptContext(context.Background())).To(Equal(context.Background()))
			})
		})
		When("there are ephemeral keys in the keyring", func() {
			When("any keys are invalid", func() {
				It("should fail", func() {
					kr := keyring.New(goodKey1, badKey1, goodKey2, badKey2, skipKey1, skipKey2)
					_, err := session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())
					_, err = session.NewClientChallenge(kr)
					Expect(err).To(HaveOccurred())

					kr = keyring.New(goodKey1, badKey1, goodKey1, skipKey1, skipKey2)
					_, err = session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())
					_, err = session.NewClientChallenge(kr)
					Expect(err).To(HaveOccurred())

					kr = keyring.New(goodKey1, badKey1, goodKey2, skipKey1, skipKey2)
					_, err = session.NewServerChallenge(kr)
					Expect(err).To(HaveOccurred())
					_, err = session.NewClientChallenge(kr)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		When("all keys are valid", func() {
			It("should succeed", func() {
				kr := keyring.New(goodKey1, goodKey2, skipKey1, skipKey2)
				_, err := session.NewServerChallenge(kr)
				Expect(err).NotTo(HaveOccurred())
				_, err = session.NewClientChallenge(kr)
				Expect(err).NotTo(HaveOccurred())

			})
			It("should have the correct attributes", func() {
				kr := keyring.New(goodKey1, goodKey2, skipKey1, skipKey2)
				c, err := session.NewServerChallenge(kr)
				Expect(err).NotTo(HaveOccurred())
				Expect(c.Attributes()).To(ConsistOf("good1", "good2"))
				_, err = session.NewClientChallenge(kr)
				Expect(err).NotTo(HaveOccurred())
				Expect(c.Attributes()).To(ConsistOf("good1", "good2"))
			})
		})
	})
	Context("Challenge Handlers", func() {
		When("the client has no attribute keys in its keyring", func() {
			It("should have no attributes in the server stream context", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(keyring.New(testSharedKeys))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Eventually(attrs).Should(Receive(HaveLen(0)))
			})
		})
		When("the client has attribute keys in its keyring", func() {
			It("should add attributes to the server stream context", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(testKeyring) // has 2 attribute keys
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Eventually(attrs).Should(Receive(HaveLen(2)))
			})
		})
		When("the client requests attributes the server does not have keys for", func() {
			It("should error", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				cc, err := dial(testKeyring.Merge(keyring.New(goodKey1)))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(attrs).NotTo(Receive())
			})
		})
		When("the client's attribute request exceeds the metadata size limit", func() {
			It("should error", func() {
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					return nil
				}))
				keys := make([]any, 0, 50)
				for i := 0; i < cap(keys); i++ {
					k := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
						session.AttributeLabelKey: fmt.Sprintf("very-large-attribute-name-to-exceed-the-metadata-size-limit-of-eight-thousand-one-hundred-and-ninety-two-bytes-%d", i),
					})
					keys = append(keys, k)
				}
				bigKeyring := keyring.New(append([]any{testSharedKeys}, keys...)...)
				cc, err := dial(bigKeyring)
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Internal, Equal("header list size to send violates the maximum size (8192 bytes) set by server")))
			})
		})
		When("the client requests a very large attribute name", func() {
			It("should error", func() {
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					return nil
				}))
				bigKey := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
					session.AttributeLabelKey: strings.Repeat("a", 8192),
				})
				cc, err := dial(testKeyring.Merge(keyring.New(bigKey)))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Internal, Equal("header list size to send violates the maximum size (8192 bytes) set by server")))
			})
		})
		When("the client requests more attributes than the server allows", func() {
			It("should error", func() {
				// limited to 200 above
				keys := make([]any, 0, 51)
				for i := 0; i < cap(keys); i++ {
					k := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
						session.AttributeLabelKey: fmt.Sprintf("attribute-%d", i),
					})
					keys = append(keys, k)
				}
				bigKeyring := keyring.New(append([]any{testSharedKeys}, keys...)...)
				cc, err := dial(testKeyring.Merge(bigKeyring))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, Equal("number of attribute requests exceeds limit")))
			})
		})
		When("the client fails an authentication challenge", func() {
			It("should error", func() {
				attrs := make(chan []session.Attribute, 1)
				serverHandler.Store(lo.ToPtr(func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				}))
				// add a mismatched key to the client keyring
				badAttrKey2 := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
					session.AttributeLabelKey: "example-session-attribute-2",
				})
				cc, err := dial(keyring.New(testSharedKeys, sessionAttrKey1, badAttrKey2))
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(attrs).NotTo(Receive())
			})
		})
	})
	Context("error handling", func() {
		Context("server challenge", func() {
			When("no metadata is found in the incoming context", func() {
				It("should return an InvalidArgument error", func() {
					ctx := context.Background()
					ctx = context.WithValue(ctx, cluster.ClusterIDKey, "foo")
					ctx = context.WithValue(ctx, cluster.SharedKeysKey, testSharedKeys)

					m := mock_grpc.NewMockServerStream(ctrl)
					m.EXPECT().Context().Return(ctx).AnyTimes()

					sc, err := session.NewServerChallenge(testKeyring)
					Expect(err).NotTo(HaveOccurred())
					out, err := sc.DoChallenge(m)
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, ContainSubstring("metadata not found")))
					Expect(out).To(BeNil())
				})
			})
			When("an error occurs sending the challenge requests", func() {
				It("should return an error", func() {
					ctx := context.Background()
					ctx = context.WithValue(ctx, cluster.ClusterIDKey, "foo")
					ctx = context.WithValue(ctx, cluster.SharedKeysKey, testSharedKeys)

					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
						session.AttributeMetadataKey, "example-session-attribute-1",
						session.AttributeMetadataKey, "example-session-attribute-2",
					))

					m := mock_grpc.NewMockServerStream(ctrl)
					m.EXPECT().Context().Return(ctx).AnyTimes()
					m.EXPECT().SendMsg(gomock.Any()).Return(errors.New("send error"))

					sc, err := session.NewServerChallenge(testKeyring)
					Expect(err).NotTo(HaveOccurred())
					out, err := sc.DoChallenge(m)
					Expect(err).To(testutil.MatchStatusCode(codes.Unknown, ContainSubstring("send error")))
					Expect(out).To(BeNil())
				})
			})
			When("an error occurs receiving the challenge responses", func() {
				It("should return an error", func() {
					ctx := context.Background()
					ctx = context.WithValue(ctx, cluster.ClusterIDKey, "foo")
					ctx = context.WithValue(ctx, cluster.SharedKeysKey, testSharedKeys)

					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
						session.AttributeMetadataKey, "example-session-attribute-1",
						session.AttributeMetadataKey, "example-session-attribute-2",
					))

					m := mock_grpc.NewMockServerStream(ctrl)
					m.EXPECT().Context().Return(ctx).AnyTimes()
					m.EXPECT().SendMsg(gomock.Any()).Return(nil)
					m.EXPECT().RecvMsg(gomock.Any()).Return(errors.New("recv error"))

					sc, err := session.NewServerChallenge(testKeyring)
					Expect(err).NotTo(HaveOccurred())
					out, err := sc.DoChallenge(m)
					Expect(err).To(testutil.MatchStatusCode(codes.Unknown, ContainSubstring("recv error")))
					Expect(out).To(BeNil())
				})
			})
			When("the client sends an invalid number of responses", func() {
				It("should return an InvalidArgument error", func() {
					ctx := context.Background()
					ctx = context.WithValue(ctx, cluster.ClusterIDKey, "foo")
					ctx = context.WithValue(ctx, cluster.SharedKeysKey, testSharedKeys)

					ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(
						session.AttributeMetadataKey, "example-session-attribute-1",
						session.AttributeMetadataKey, "example-session-attribute-2",
					))

					m := mock_grpc.NewMockServerStream(ctrl)
					m.EXPECT().Context().Return(ctx).AnyTimes()
					m.EXPECT().SendMsg(gomock.Any()).Return(nil)
					m.EXPECT().RecvMsg(gomock.Any()).DoAndReturn(func(m interface{}) error {
						m.(*corev1.ChallengeResponseList).Items = []*corev1.ChallengeResponse{
							{},
							{},
							{},
						}
						return nil
					})

					sc, err := session.NewServerChallenge(testKeyring)
					Expect(err).NotTo(HaveOccurred())
					out, err := sc.DoChallenge(m)
					Expect(err).To(testutil.MatchStatusCode(codes.InvalidArgument, ContainSubstring("invalid number of challenge responses")))
					Expect(out).To(BeNil())
				})
			})
		})
		When("an error occurs sending session info to the client", func() {
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
								SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.SessionInfo{}))).
								DoAndReturn(ss.SendMsg).
								AnyTimes(),
							m.EXPECT().
								SendMsg(gomock.AssignableToTypeOf(&corev1.SessionInfo{})).
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
				cc, err := dial()
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Unknown, Equal("test error")))
			})
		})
		When("the session info does not validate", func() {
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
								SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.SessionInfo{}))).
								DoAndReturn(ss.SendMsg).
								AnyTimes(),
							m.EXPECT().
								SendMsg(gomock.AssignableToTypeOf(&corev1.SessionInfo{})).
								DoAndReturn(func(msg interface{}) error {
									si := msg.(*corev1.SessionInfo)
									si.Mac[0] ^= 0x01
									return ss.SendMsg(msg)
								}).
								Times(1),
						)
						return handler(srv, m)
					},
				}
				DeferCleanup(func() {
					serverInterceptors = nil
				})
			})
			It("should return an Aborted error", func() {
				cc, err := dial()
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, Equal("session info verification failed")))
			})
		})
		When("the server sends the wrong number of challenge requests", func() {
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
								SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.ChallengeRequestList{}))).
								DoAndReturn(ss.SendMsg).
								AnyTimes(),
							m.EXPECT().
								SendMsg(gomock.AssignableToTypeOf(&corev1.ChallengeRequestList{})).
								DoAndReturn(ss.SendMsg).
								Times(1),
							m.EXPECT().
								SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.ChallengeRequestList{}))).
								DoAndReturn(ss.SendMsg).
								AnyTimes(),
							m.EXPECT().
								SendMsg(gomock.AssignableToTypeOf(&corev1.ChallengeRequestList{})).
								DoAndReturn(func(msg interface{}) error {
									crl := msg.(*corev1.ChallengeRequestList)
									crl.Items = append(crl.Items, &corev1.ChallengeRequest{})
									return ss.SendMsg(msg)
								}).
								Times(1),
						)
						return handler(srv, m)
					},
				}
				DeferCleanup(func() {
					serverInterceptors = nil
				})
			})
			It("should return an Internal error", func() {
				cc, err := dial()
				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Internal, Equal("server sent the wrong number of challenge requests")))
			})
		})
		When("an error occurs when sending the challenge responses", func() {
			It("should return an error", func() {
				clientChallenge, err := session.NewClientChallenge(testKeyring)
				Expect(err).NotTo(HaveOccurred())

				handler := challenges.Chained(
					testutil.Must(authv2.NewClientChallenge(testKeyring, "foo", test.Log)),
					challenges.If(clientChallenge.HasAttributes).Then(clientChallenge),
				)

				cc, err := grpc.Dial("bufconn", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()),
					grpc.WithChainStreamInterceptor(
						cluster.StreamClientInterceptor(handler), // order is important for wrapping the client stream
						func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
							stream, _ := streamer(ctx, desc, cc, method, opts...)

							mockClientStream := mock_grpc.NewMockClientStream(ctrl)
							mockClientStream.EXPECT().Context().DoAndReturn(stream.Context).AnyTimes()
							mockClientStream.EXPECT().RecvMsg(gomock.Any()).DoAndReturn(stream.RecvMsg).AnyTimes()
							gomock.InOrder(
								mockClientStream.EXPECT().
									SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.ChallengeResponseList{}))).
									DoAndReturn(stream.SendMsg).
									AnyTimes(),
								mockClientStream.EXPECT().
									SendMsg(gomock.AssignableToTypeOf(&corev1.ChallengeResponseList{})).
									DoAndReturn(stream.SendMsg).
									Times(1),
								mockClientStream.EXPECT().
									SendMsg(gomock.Not(gomock.AssignableToTypeOf(&corev1.ChallengeResponseList{}))).
									DoAndReturn(stream.SendMsg).
									AnyTimes(),
								mockClientStream.EXPECT().
									SendMsg(gomock.AssignableToTypeOf(&corev1.ChallengeResponseList{})).
									Return(errors.New("send error")).
									Times(1),
							)

							return mockClientStream, nil
						},
					),
				)

				Expect(err).NotTo(HaveOccurred())
				defer cc.Close()

				client := testgrpc.NewStreamServiceClient(cc)
				_, err = client.Stream(context.Background())
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, ContainSubstring("error sending challenge response")))
			})
			It("should return an error", func() {
				// another way to implement the previous test, for posterity
				clientChallenge, err := session.NewClientChallenge(testKeyring)
				Expect(err).NotTo(HaveOccurred())

				ctx := context.Background()
				ctx = context.WithValue(ctx, cluster.ClusterIDKey, "foo")
				ctx = context.WithValue(ctx, cluster.SharedKeysKey, testSharedKeys)

				mockClientStream := mock_grpc.NewMockClientStream(ctrl)
				mockClientStream.EXPECT().Context().Return(ctx).AnyTimes()
				mockClientStream.EXPECT().RecvMsg(gomock.Any()).DoAndReturn(func(msg interface{}) error {
					m := msg.(*corev1.ChallengeRequestList)
					m.Items = []*corev1.ChallengeRequest{
						{Challenge: make([]byte, 32)},
						{Challenge: make([]byte, 32)},
					}
					return nil
				})
				mockClientStream.EXPECT().
					SendMsg(gomock.Any()).
					Return(errors.New("send error")).
					Times(1)

				_, err = clientChallenge.DoChallenge(mockClientStream)
				Expect(err).To(testutil.MatchStatusCode(codes.Aborted, ContainSubstring("error sending challenge response")))
			})
		})
	})
	Context("PerRPCCredentials", func() {
		It("should allow using session attributes as PerRPCCredentials", func() {
			server := grpc.NewServer(
				grpc.Creds(insecure.NewCredentials()),
				grpc.ChainStreamInterceptor(
					func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
						stream := streams.NewServerStreamWithContext(ss)
						stream.Ctx = session.AttributesKey.FromIncomingCredentials(stream.Ctx)
						return handler(srv, stream)
					},
				),
			)

			attrs := make(chan []session.Attribute, 1)
			testgrpc.RegisterStreamServiceServer(server, &testgrpc.StreamServer{
				ServerHandler: func(stream testgrpc.StreamService_StreamServer) error {
					attrs <- session.StreamAuthorizedAttributes(stream.Context())
					return nil
				},
			})
			listener := bufconn.Listen(1024 * 1024)
			go server.Serve(listener)
			DeferCleanup(listener.Close)

			cc, _ := grpc.Dial("bufconn",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithPerRPCCredentials(session.AttributesKey),
			)

			client := testgrpc.NewStreamServiceClient(cc)
			{
				ctx := context.WithValue(context.Background(), session.AttributesKey, []session.Attribute{
					session.NewAttribute("foo"),
					session.NewAttribute("bar"),
				})

				_, err := client.Stream(ctx)
				Expect(err).NotTo(HaveOccurred())

				var attrNames []string
				for _, attr := range <-attrs {
					attrNames = append(attrNames, attr.Name())
				}
				Expect(attrNames).To(ConsistOf("foo", "bar"))
			}
			{
				ctx := context.WithValue(context.Background(), session.AttributesKey, []session.SecretAttribute{
					testutil.Must(session.NewSecretAttribute("baz", make([]byte, 32))),
				})

				_, err := client.Stream(ctx)
				Expect(err).NotTo(HaveOccurred())

				var attrNames []string
				for _, attr := range <-attrs {
					attrNames = append(attrNames, attr.Name())
				}
				Expect(attrNames).To(ConsistOf("baz"))
			}
			{
				ctx := context.WithValue(context.Background(), session.AttributesKey, []string{
					"wrong-type",
				})

				_, err := client.Stream(ctx)
				Expect(err).To(testutil.MatchStatusCode(codes.Unauthenticated,
					Equal("transport: per-RPC creds failed due to error: invalid session attribute type []string")))
			}
			{
				_, err := client.Stream(context.Background())
				Expect(err).NotTo(HaveOccurred())

				Expect(<-attrs).To(BeEmpty())
			}
		})
		Specify("misc error checking", func() {
			ctx := session.AttributesKey.FromIncomingCredentials(context.Background())
			Expect(ctx).To(Equal(context.Background()))

			attrs := session.StreamAuthorizedAttributes(
				context.WithValue(context.Background(), session.AttributesKey, []string{"foo"}))
			Expect(attrs).To(BeNil())
		})
	})
})

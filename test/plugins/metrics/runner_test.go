package metrics_test

import (
	"fmt"
	"math"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("Target Runner", Ordered, Label("unit"), func() {
	var (
		// todo: set up a mock prometheus endpoint since we no longer handle readers
		addr         string
		runner       agent.TargetRunner
		writerClient *mockRemoteWriteClient
		target       *remoteread.Target

		query = &remoteread.Query{
			StartTimestamp: &timestamppb.Timestamp{},
			EndTimestamp: &timestamppb.Timestamp{
				Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
			},
			Matchers: nil,
		}
	)

	BeforeAll(func() {
		By("adding a remote read  server")
		addr = fmt.Sprintf("127.0.0.1:%d", freeport.GetFreePort())

		server := http.Server{
			Addr:    addr,
			Handler: NewReadHandler(),
		}

		go func() {
			server.ListenAndServe()
		}()
		DeferCleanup(server.Close)

		Eventually(func() error {
			_, err := (&http.Client{}).Get(fmt.Sprintf("http://%s/health", addr))
			return err
		}).Should(Not(HaveOccurred()))
	})

	BeforeEach(func() {
		lg := logger.NewPluginLogger().Named("test-runner")

		writerClient = &mockRemoteWriteClient{}

		runner = agent.NewTargetRunner(lg)
		runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
			return writerClient
		}))

		target = &remoteread.Target{
			Meta: &remoteread.TargetMeta{
				Name:      "testTarget",
				ClusterId: "testCluster",
			},
			Spec: &remoteread.TargetSpec{
				Endpoint: "http://127.0.0.1:9090/api/v1/read",
			},
			Status: nil,
		}

	})

	When("target status is not running", func() {
		It("cannot get status", func() {
			status, err := runner.GetStatus("test")
			AssertTargetStatus(&remoteread.TargetStatus{
				Progress: nil,
				Message:  "",
				State:    remoteread.TargetState_NotRunning,
			}, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("cannot stop", func() {
			err := runner.Stop("test")
			Expect(err).To(HaveOccurred())
		})
	})

	When("target runner cannot reach target endpoint", func() {
		It("should retry until success", func() {
			target.Spec.Endpoint = "http://i.do.not.exist:9090/api/v1/read"

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			status, err := runner.GetStatus(target.Meta.Name)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(time.Second)

			Expect(status.State).To(Equal(remoteread.TargetState_Running))
		})
	})

	When("target runner can reach target endpoint", func() {
		It("should complete", func() {
			target.Spec.Endpoint = fmt.Sprintf("http://%s/small", addr)

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Completed))

			Eventually(func() string {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.Message
			}).Should(Equal("completed"))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "completed",
				State:   remoteread.TargetState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})

		It("should complete with large payload", func() {
			target.Spec.Endpoint = fmt.Sprintf("http://%s/large", addr)

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Completed))

			Eventually(func() string {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.Message
			}).Should(Equal("completed"))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "completed",
				State:   remoteread.TargetState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(4))
		})
	})

	When("target is stopped during push", func() {
		It("should be marked as stopped", func() {
			target.Spec.Endpoint = fmt.Sprintf("http://%s/small", addr)

			runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
				return &mockRemoteWriteClient{
					Delay: math.MaxInt64,
				}
			}))

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Running))

			err = runner.Stop(target.Meta.Name)
			Expect(err).NotTo(HaveOccurred())

			// status dosn't update immediately so we need to extend the defualt timeout
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Canceled))
		})
	})

	When("target pushes with unrecoverable error", func() {
		It("should fail", func() {
			target.Spec.Endpoint = fmt.Sprintf("http://%s/small", addr)

			runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
				return &mockRemoteWriteClient{
					Error: fmt.Errorf("some unrecoverable error"),
				}
			}))

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Failed))
		})
	})
})

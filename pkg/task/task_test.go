package task_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/util"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"

	"github.com/rancher/opni/pkg/task"
)

var _ = Describe("Task", Ordered, Label("unit"), func() {
	var store task.KVStore

	BeforeAll(func() {
		store = inmemory.NewKeyValueStore[*corev1.TaskStatus](util.ProtoClone)
	})

	newController := func(ctx context.Context, ch chan any) *task.Controller {
		controller, err := task.NewController(ctx, "test", store, &SampleTaskRunner{
			Input: ch,
		})
		Expect(err).NotTo(HaveOccurred())
		return controller
	}

	When("a task runs successfully", func() {
		It("should end in the completed state", func() {
			input := make(chan any, 1)
			controller := newController(context.Background(), input)
			input <- "foo"

			endState := make(chan task.State, 1)
			err := controller.LaunchTask("test1", task.WithMetadata(SampleTaskConfig{
				Limit: 1,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())
			Eventually(endState).Should(Receive(Equal(corev1.TaskState_Completed)))
		})
	})
	When("a task encounters an error", func() {
		It("should end in the failed state", func() {
			input := make(chan any, 2)
			controller := newController(context.Background(), input)
			input <- "foo"
			close(input)

			endState := make(chan task.State, 1)
			err := controller.LaunchTask("test2", task.WithMetadata(SampleTaskConfig{
				Limit: 2,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())
			Eventually(endState).Should(Receive(Equal(corev1.TaskState_Failed)))
		})
	})
	When("a task is canceled partway through", func() {
		It("should preserve its state", func() {
			By("starting the task")
			input := make(chan any)
			controller := newController(context.Background(), input)
			endState := make(chan task.State, 1)
			err := controller.LaunchTask("test3", task.WithMetadata(SampleTaskConfig{
				Limit: 2,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())
			By("partially completing the task")
			Eventually(input).Should(BeSent("foo"))
			Consistently(input).Should(BeEmpty())

			By("canceling the task")
			controller.CancelTask("test3")
			select {
			case state := <-endState:
				Expect(state).To(Equal(corev1.TaskState_Canceled))
			case <-time.After(10 * time.Second):
				Fail("timeout waiting for task to cancel")
			}

			By("checking the task's state in the key-value store")
			status, err := store.Get(context.Background(), "/tasks/test/test3")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.State).To(Equal(corev1.TaskState_Canceled))
			Expect(status.Progress.GetCurrent()).To(BeEquivalentTo(1))
			Expect(status.Progress.GetTotal()).To(BeEquivalentTo(2))

			By("restarting the task by using the same ID")
			err = controller.LaunchTask("test3", task.WithMetadata(SampleTaskConfig{
				Limit: 2,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())

			By("completing the remainder of the task")
			Eventually(input).Should(BeSent("bar"))
			Consistently(input).Should(BeEmpty())

			By("ensuring the task ends in the completed state")
			select {
			case state := <-endState:
				Expect(state).To(Equal(corev1.TaskState_Completed))
			case <-time.After(10 * time.Second):
				Fail("timeout waiting for task to complete")
			}

			By("checking the task's state in the key-value store")
			status, err = store.Get(context.Background(), "/tasks/test/test3")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.State).To(Equal(corev1.TaskState_Completed))
			Expect(status.Progress.GetCurrent()).To(BeEquivalentTo(2))
			Expect(status.Progress.GetTotal()).To(BeEquivalentTo(2))
		})
	})
	When("a task fails and is restarted with the same ID", func() {
		It("should retain logs, but not state", func() {
			By("starting the task")
			input := make(chan any)
			runner := &SampleTaskRunner{
				Input: input,
			}
			controller, err := task.NewController(context.Background(), "test", store, runner)
			Expect(err).NotTo(HaveOccurred())

			endState := make(chan task.State, 1)

			err = controller.LaunchTask("test4", task.WithMetadata(SampleTaskConfig{
				Limit: 2,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())

			By("completing half the task")
			Eventually(input).Should(BeSent("foo"))

			By("closing the input channel")
			close(input)

			By("ensuring the task ends in the failed state")
			select {
			case state := <-endState:
				Expect(state).To(Equal(corev1.TaskState_Failed))
			case <-time.After(10 * time.Second):
				Fail("timeout waiting for task to fail")
			}

			By("restarting the task by using the same ID")
			input = make(chan any)
			runner.Input = input
			err = controller.LaunchTask("test4", task.WithMetadata(SampleTaskConfig{
				Limit: 2,
			}), task.WithStateCallback(endState))
			Expect(err).NotTo(HaveOccurred())

			By("completing half the task")
			Eventually(input).Should(BeSent("bar"))

			By("closing the input channel")
			close(input)

			By("ensuring the task ends in the failed state")
			select {
			case state := <-endState:
				Expect(state).To(Equal(corev1.TaskState_Failed))
			case <-time.After(10 * time.Second):
				Fail("timeout waiting for task to fail")
			}

			By("checking the task's state in the key-value store")
			status, err := store.Get(context.Background(), "/tasks/test/test4")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.State).To(Equal(corev1.TaskState_Failed))
			Expect(status.Progress.GetCurrent()).To(BeEquivalentTo(1))
			Expect(status.Progress.GetTotal()).To(BeEquivalentTo(2))
		})
	})
})

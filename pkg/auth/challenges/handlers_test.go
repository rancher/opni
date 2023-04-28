package challenges_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/mock/auth"
	"github.com/rancher/opni/pkg/util/streams"
	"google.golang.org/grpc/metadata"

	"github.com/rancher/opni/pkg/auth/challenges"
)

type (
	conditionChallengeKeyType string
)

const (
	conditionChallengeKey conditionChallengeKeyType = "key"
)

type mockStream struct {
	streams.Stream
}

func (m *mockStream) Context() context.Context {
	return context.Background()
}

var _ = Describe("Handlers", Label("unit"), func() {
	Context("Conditional Handler", func() {
		ifTrueH := mock_auth.NewTestChallengeHandler(func(s streams.Stream) (context.Context, error) {
			return context.WithValue(s.Context(), conditionChallengeKey, true), nil
		}, "condition", "true")
		ifFalseH := mock_auth.NewTestChallengeHandler(func(s streams.Stream) (context.Context, error) {
			return context.WithValue(s.Context(), conditionChallengeKey, false), nil
		}, "condition", "false")
		// errH := test.NewTestChallengeHandler(func(s streams.Stream) (context.Context, error) {
		// 	return nil, errors.New("error")
		// })
		trueCondition := func(s context.Context) (bool, error) {
			return true, nil
		}
		falseCondition := func(s context.Context) (bool, error) {
			return false, nil
		}
		errCondition := func(s context.Context) (bool, error) {
			return false, errors.New("error")
		}

		When("creating a new conditional handler with true and false cases", func() {
			When("the condition func returns true", func() {
				It("should call the handler for the true case", func() {
					h := challenges.If(trueCondition).Then(ifTrueH).Else(ifFalseH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Value(conditionChallengeKey)).To(BeTrue())
				})
			})
			It("should conditionally intercept contexts", func() {
				h := challenges.If(trueCondition).Then(ifTrueH).Else(ifFalseH)
				ctx := h.InterceptContext(context.Background())
				md, ok := metadata.FromOutgoingContext(ctx)
				Expect(ok).To(BeTrue())
				Expect(md.Get("condition")).To(Equal([]string{"true"}))
			})
			When("the condition func returns false", func() {
				It("should call the handler for the false case", func() {
					h := challenges.If(falseCondition).Then(ifTrueH).Else(ifFalseH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Value(conditionChallengeKey)).To(BeFalse())
				})
				It("should conditionally intercept contexts", func() {
					h := challenges.If(falseCondition).Then(ifTrueH).Else(ifFalseH)
					ctx := h.InterceptContext(context.Background())
					md, ok := metadata.FromOutgoingContext(ctx)
					Expect(ok).To(BeTrue())
					Expect(md.Get("condition")).To(Equal([]string{"false"}))
				})
			})
			When("the condition func returns an error", func() {
				It("should call neither case, and return the error", func() {
					h := challenges.If(errCondition).Then(ifTrueH).Else(ifFalseH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).To(HaveOccurred())
					Expect(ctx).To(BeNil())
				})
				It("should conditionally intercept contexts", func() {
					h := challenges.If(errCondition).Then(ifTrueH).Else(ifFalseH)
					ctx := h.InterceptContext(context.Background())
					Expect(ctx).To(Equal(context.Background()))
				})
			})
			When("no conditions are provided", func() {
				It("should do nothing", func() {
					h := challenges.If(trueCondition).Then(nil)
					ctx := h.InterceptContext(context.Background())
					Expect(ctx).To(Equal(context.Background()))
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx).To(Equal(context.Background()))
				})
			})
		})
		When("creating a new conditional handler with only a true case", func() {
			When("the condition func returns true", func() {
				It("should call the handler for the true case", func() {
					h := challenges.If(trueCondition).Then(ifTrueH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx.Value(conditionChallengeKey)).To(BeTrue())
				})
			})
			When("the condition func returns false", func() {
				It("should do nothing and return no error", func() {
					h := challenges.If(falseCondition).Then(ifTrueH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).NotTo(HaveOccurred())
					Expect(ctx).To(Equal(context.Background()))
				})
			})
			When("the condition func returns an error", func() {
				It("should call neither case, and return the error", func() {
					h := challenges.If(errCondition).Then(ifTrueH)
					ctx, err := h.DoChallenge(&mockStream{})
					Expect(err).To(HaveOccurred())
					Expect(ctx).To(BeNil())
				})
			})
		})
	})
	Context("Chained Handler", func() {
		It("should call all handlers in order", func() {
			handlers := []challenges.ChallengeHandler{}
			for i := 0; i < 100; i++ {
				i := i
				handlers = append(handlers, mock_auth.NewTestChallengeHandler(func(s streams.Stream) (context.Context, error) {
					return context.WithValue(s.Context(), conditionChallengeKeyType(fmt.Sprint(i)), i), nil
				}, "handler-indexes", fmt.Sprint(i)))
			}
			chained := challenges.Chained(handlers...)
			ctx := context.Background()
			ctx = chained.InterceptContext(ctx)

			md, ok := metadata.FromOutgoingContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(md).To(HaveKey("handler-indexes"))

			values := md.Get("handler-indexes")
			Expect(values).To(HaveLen(100))
			for i, v := range values {
				Expect(v).To(Equal(fmt.Sprint(i)))
			}

			outCtx, err := chained.DoChallenge(&mockStream{})
			Expect(err).NotTo(HaveOccurred())
			for i := 0; i < 100; i++ {
				Expect(outCtx.Value(conditionChallengeKeyType(fmt.Sprint(i)))).To(Equal(i))
			}
		})
		When("one of the handlers returns an error", func() {
			It("should abort and return the error", func() {
				handlers := []challenges.ChallengeHandler{}
				for i := 0; i < 100; i++ {
					i := i
					handlers = append(handlers, mock_auth.NewTestChallengeHandler(func(s streams.Stream) (context.Context, error) {
						if i == 50 {
							return nil, errors.New("error")
						}
						return context.WithValue(s.Context(), conditionChallengeKeyType(fmt.Sprint(i)), i), nil
					}, "handler-indexes", fmt.Sprint(i)))
				}
				chained := challenges.Chained(handlers...)
				ctx, err := chained.DoChallenge(&mockStream{})
				Expect(err).To(MatchError("error"))
				Expect(ctx).To(BeNil())
			})
		})
	})
})

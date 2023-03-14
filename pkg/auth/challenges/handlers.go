package challenges

import (
	"context"

	"github.com/rancher/opni/pkg/util/streams"
)

type ChallengeHandler interface {
	DoChallenge(stream streams.Stream) (context.Context, error)
	InterceptContext(ctx context.Context) context.Context
}

type conditional struct {
	condition func(streamContext context.Context) (bool, error)
	ifTrue    ChallengeHandler
	ifFalse   ChallengeHandler
}

type conditionalThen interface {
	Then(challenge ChallengeHandler) conditionalElse
}

type conditionalElse interface {
	ChallengeHandler
	Else(challenge ChallengeHandler) ChallengeHandler
}

func (c *conditional) DoChallenge(cs streams.Stream) (context.Context, error) {
	if v, err := c.condition(cs.Context()); err != nil {
		return nil, err
	} else if v && c.ifTrue != nil {
		return c.ifTrue.DoChallenge(cs)
	} else if c.ifFalse != nil {
		return c.ifFalse.DoChallenge(cs)
	}
	return cs.Context(), nil
}

func (c *conditional) InterceptContext(ctx context.Context) context.Context {
	if v, err := c.condition(ctx); err != nil {
		return ctx
	} else if v && c.ifTrue != nil {
		return c.ifTrue.InterceptContext(ctx)
	} else if c.ifFalse != nil {
		return c.ifFalse.InterceptContext(ctx)
	}
	return ctx
}

type ConditionFunc = func(streamContext context.Context) (bool, error)

func If(condition ConditionFunc) conditionalThen {
	return &conditional{
		condition: condition,
	}
}

func (c *conditional) Then(challenge ChallengeHandler) conditionalElse {
	c.ifTrue = challenge
	return c
}

func (c *conditional) Else(challenge ChallengeHandler) ChallengeHandler {
	c.ifFalse = challenge
	return c
}

type chained struct {
	challenges []ChallengeHandler
}

func Chained(challenges ...ChallengeHandler) ChallengeHandler {
	return &chained{
		challenges: challenges,
	}
}

func (c *chained) DoChallenge(cs streams.Stream) (context.Context, error) {
	s := streams.NewStreamWithContext(cs.Context(), cs)
	for _, challenge := range c.challenges {
		out, err := challenge.DoChallenge(s)
		if err != nil {
			return nil, err
		}
		s.SetContext(out)
	}
	return s.Context(), nil
}

func (c *chained) InterceptContext(ctx context.Context) context.Context {
	for _, challenge := range c.challenges {
		ctx = challenge.InterceptContext(ctx)
	}
	return ctx
}

type HandlerFunc func(streams.Stream) (context.Context, error)

package testutil

import (
	"fmt"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"google.golang.org/protobuf/proto"
)

func ProtoEqual(expected proto.Message) types.GomegaMatcher {
	return &ProtoMatcher{
		Expected: expected,
	}
}

type ProtoMatcher struct {
	Expected proto.Message
}

func (matcher *ProtoMatcher) Match(actual any) (success bool, err error) {
	if actual == nil && matcher.Expected == nil {
		return false, fmt.Errorf("Refusing to compare <nil> to <nil>.\nBe explicit and use BeNil() instead.  This is to avoid mistakes where both sides of an assertion are erroneously uninitialized.")
	}
	if _, ok := actual.(proto.Message); !ok {
		return false, fmt.Errorf("ProtoMatcher expects a proto.Message. Got:\n%s", format.Object(actual, 1))
	}
	return proto.Equal(actual.(proto.Message), matcher.Expected), nil
}

func (matcher *ProtoMatcher) FailureMessage(actual any) (message string) {
	return format.Message(actual, "to equal", matcher.Expected)
}

func (matcher *ProtoMatcher) NegatedFailureMessage(actual any) (message string) {
	return format.Message(actual, "not to equal", matcher.Expected)
}

package testutil

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
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
		return false, fmt.Errorf("Refusing to compare <nil> to <nil>.\nBe explicit and use BeNil() instead.  This is to avoid mistakes where both sides of an assertion are erroneously uninitialized")
	}
	if _, ok := actual.(proto.Message); !ok {
		return false, fmt.Errorf("ProtoMatcher expects a proto.Message. Got:\n%s", format.Object(actual, 1))
	}
	return proto.Equal(actual.(proto.Message), matcher.Expected), nil
}

func (matcher *ProtoMatcher) FailureMessage(actual any) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s",
		format.IndentString(prototext.Format(actual.(proto.Message)), 1),
		"to equal",
		format.IndentString(prototext.Format(matcher.Expected), 1),
	)
}

func (matcher *ProtoMatcher) NegatedFailureMessage(actual any) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s",
		format.IndentString(prototext.Format(actual.(proto.Message)), 1),
		"not to equal",
		format.IndentString(prototext.Format(matcher.Expected), 1),
	)
}

type StatusCodeMatcher struct {
	Expected any
	matchMsg types.GomegaMatcher
}

func MatchStatusCode(expected any, matchMessage ...types.GomegaMatcher) types.GomegaMatcher {
	m := &StatusCodeMatcher{
		Expected: expected,
	}
	if len(matchMessage) > 0 {
		m.matchMsg = matchMessage[0]
	}
	return m
}

func code(value any) codes.Code {
	if value == nil {
		return codes.OK
	}
	switch value := value.(type) {
	case error:
		return status.Code(value)
	case *status.Status:
		return value.Code()
	case codes.Code:
		return value
	case uint32:
		return codes.Code(value)
	default:
		panic(fmt.Sprintf("MatchStatus expects a grpc status, error, or codes.Code. Got:\n%s", format.Object(value, 1)))
	}
}

func message(value any) string {
	if value == nil {
		return ""
	}
	switch value := value.(type) {
	case error:
		return status.Convert(value).Message()
	case *status.Status:
		return value.Message()
	case codes.Code:
		return value.String()
	case uint32:
		return codes.Code(value).String()
	default:
		panic(fmt.Sprintf("MatchStatus expects a grpc status, error, or codes.Code. Got:\n%s", format.Object(value, 1)))
	}
}

func (m *StatusCodeMatcher) Match(actual any) (success bool, err error) {
	if actual == nil && m.Expected == nil {
		return false, fmt.Errorf("Refusing to compare <nil> to <nil>.\nBe explicit and use BeNil() instead.  This is to avoid mistakes where both sides of an assertion are erroneously uninitialized")
	}
	if code(actual) != code(m.Expected) {
		return false, nil
	}
	if m.matchMsg != nil {
		return m.matchMsg.Match(message(actual))
	}
	return true, nil
}

func (m *StatusCodeMatcher) FailureMessage(actual any) (message string) {
	actualStatusCode := code(actual)
	expectedStatusCode := code(m.Expected)

	actualMsg := fmt.Sprintf("%s | %s(%d)", format.Object(actual, 1), actualStatusCode.String(), actualStatusCode)
	expectedMsg := fmt.Sprintf("%s | %s(%d)", format.Object(m.Expected, 1), expectedStatusCode.String(), expectedStatusCode)

	return fmt.Sprintf("Expected\n%s\nto match the status code of\n%s", actualMsg, expectedMsg)
}

func (m *StatusCodeMatcher) NegatedFailureMessage(actual any) (message string) {
	msg := m.FailureMessage(actual)
	return strings.Replace(msg, "to match", "not to match", 1)
}

package testutil

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
)

func ProtoEqual(expected proto.Message) *ProtoMatcher {
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
	diff := cmp.Diff(actual.(proto.Message), matcher.Expected, protocmp.Transform())
	return fmt.Sprintf("Expected\n%s\n%s\n%s\ndiff:\n%s",
		format.IndentString(prototext.Format(actual.(proto.Message)), 1),
		"to equal",
		format.IndentString(prototext.Format(matcher.Expected), 1),
		diff,
	)
}

func (matcher *ProtoMatcher) NegatedFailureMessage(actual any) (message string) {
	diff := cmp.Diff(actual.(proto.Message), matcher.Expected, protocmp.Transform())
	return fmt.Sprintf("Expected\n%s\n%s\n%s\ndiff:\n%s",
		format.IndentString(prototext.Format(actual.(proto.Message)), 1),
		"not to equal",
		format.IndentString(prototext.Format(matcher.Expected), 1),
		diff,
	)
}

// implements gomock.Matcher
func (matcher *ProtoMatcher) String() string {
	return prototext.MarshalOptions{Multiline: false}.Format(matcher.Expected)
}

// implements gomock.Matcher
func (matcher *ProtoMatcher) Matches(x interface{}) bool {
	success, _ := matcher.Match(x)
	return success
}

func ProtoValueEqual(expected protoreflect.Value) *ProtoValueMatcher {
	return &ProtoValueMatcher{
		Expected: expected,
	}
}

type ProtoValueMatcher struct {
	Expected protoreflect.Value
}

func (matcher *ProtoValueMatcher) Match(actual any) (success bool, err error) {
	if actual == nil && !matcher.Expected.IsValid() {
		return false, fmt.Errorf("Refusing to compare <nil> to <nil>.\nBe explicit and use BeNil() instead.  This is to avoid mistakes where both sides of an assertion are erroneously uninitialized")
	}
	if actual, ok := actual.(protoreflect.Value); !ok {
		return false, fmt.Errorf("ProtoValueMatcher expects a protoreflect.Value. Got:\n%s", format.Object(actual, 1))
	} else {
		return matcher.Expected.Equal(actual), nil
	}
}

func (matcher *ProtoValueMatcher) FailureMessage(actual any) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s",
		format.IndentString(humanizeProtoreflectValue(actual.(protoreflect.Value)), 1),
		"to equal",
		format.IndentString(humanizeProtoreflectValue(matcher.Expected), 1),
	)
}

func (matcher *ProtoValueMatcher) NegatedFailureMessage(actual any) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s\n%s",
		format.IndentString(humanizeProtoreflectValue(actual.(protoreflect.Value)), 1),
		"not to equal",
		format.IndentString(humanizeProtoreflectValue(matcher.Expected), 1),
	)
}

func humanizeProtoreflectValue(v protoreflect.Value) string {
	switch v := v.Interface().(type) {
	case nil:
		return "nil"
	case bool, int32, int64, uint32, uint64, float32, float64, string:
		return fmt.Sprintf("%v", v)
	case []byte:
		return string(v)
	case protoreflect.EnumNumber:
		return fmt.Sprintf("%v", v)
	case protoreflect.Message:
		return prototext.Format(v.Interface())
	case protoreflect.List:
		builder := strings.Builder{}
		builder.WriteString("[\n")
		for i := 0; i < v.Len(); i++ {
			fmt.Fprintf(&builder, "  %s\n", humanizeProtoreflectValue(v.Get(i)))
		}
		builder.WriteString("]")
		return builder.String()
	case protoreflect.Map:
		builder := strings.Builder{}
		builder.WriteString("{\n")
		kvs := []string{}
		v.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
			kvs = append(kvs, fmt.Sprintf("%s: %s", mk.String(), humanizeProtoreflectValue(v)))
			return true
		})
		slices.Sort(kvs)
		for _, kv := range kvs {
			builder.WriteString(kv)
		}
		builder.WriteString("}")
		return builder.String()
	case protoreflect.ProtoMessage:
		panic(fmt.Sprintf("invalid proto.Message(%T) type, expected a protoreflect.Message type", v))
	default:
		panic(fmt.Sprintf("invalid type: %T", v))
	}
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

// implements gomock.Matcher
func (m *StatusCodeMatcher) String() string {
	expectedStatusCode := code(m.Expected)
	return fmt.Sprintf("%s | %s(%d)", format.Object(m.Expected, 1), expectedStatusCode.String(), expectedStatusCode)
}

// implements gomock.Matcher
func (m *StatusCodeMatcher) Matches(x interface{}) bool {
	success, _ := m.Match(x)
	return success
}

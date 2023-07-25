package flagutil

import (
	"fmt"
	"strings"

	"github.com/rancher/opni/pkg/util/merge"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"
)

func MarshalToFlags[T interface {
	proto.Message
	FlagSet(...string) *pflag.FlagSet
}](obj T, prefix ...string) []string {
	// First, create a new empty message of the same type
	newEmpty := obj.ProtoReflect().New().Interface().(T)
	// Create a flag set from the empty message. This will load the defaults
	// into the message fields.
	fs := newEmpty.FlagSet(prefix...)
	// merge the original message into the message that now has all defaults
	merge.MergeWithReplace(newEmpty, obj)
	// update the flag set to reflect the new message
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Value.String() != f.DefValue {
			switch f.Value.(type) {
			case pflag.SliceValue:
				fs.Set(f.Name, trimSliceBrackets(f.Value.String()))
			default:
				fs.Set(f.Name, f.Value.String())
			}
		}
	})
	// Now, convert to flags
	var flags []string
	fs.Visit(func(f *pflag.Flag) {
		flags = append(flags, fmt.Sprintf("--%s=%s", f.Name, trimSliceBrackets(f.Value.String())))
	})
	return flags
}

func trimSliceBrackets(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
}

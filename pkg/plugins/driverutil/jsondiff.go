package driverutil

import (
	"fmt"
	"strings"

	"github.com/nsf/jsondiff"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func MarshalConfigJson(t proto.Message) []byte {
	bytes, _ := protojson.MarshalOptions{
		// NB: EmitUnpopulated must be false, otherwise the diff algorithm will not
		// be able to distinguish removed fields from null (unpopulated) fields.
		EmitUnpopulated: false,
		UseProtoNames:   true,
	}.Marshal(t)
	return bytes
}

func RenderJsonDiff[T proto.Message](old, new T, opts jsondiff.Options) (string, bool) {
	oldJson := MarshalConfigJson(old)
	newJson := MarshalConfigJson(new)

	difference, str := jsondiff.Compare(oldJson, newJson, &opts)
	if difference == jsondiff.FullMatch {
		return "", false
	}
	return str, true
}

func DiffStat(diff string, opts ...jsondiff.Options) string {
	if diff == "" {
		return ""
	}
	options := jsondiff.DefaultConsoleOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	numAdded := strings.Count(diff, options.Added.Begin)
	numRemoved := strings.Count(diff, options.Removed.Begin)
	numChanged := strings.Count(diff, options.Changed.Begin)

	parts := []string{}
	if numAdded > 0 {
		parts = append(parts, fmt.Sprintf("+%d", numAdded))
	}
	if numRemoved > 0 {
		parts = append(parts, fmt.Sprintf("-%d", numRemoved))
	}
	if numChanged > 0 {
		parts = append(parts, fmt.Sprintf("~%d", numChanged))
	}

	return strings.Join(parts, "/")
}

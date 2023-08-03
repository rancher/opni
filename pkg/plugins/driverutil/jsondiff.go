package driverutil

import (
	"github.com/nsf/jsondiff"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func MarshalConfigJson(t proto.Message) []byte {
	bytes, _ := protojson.MarshalOptions{
		EmitUnpopulated: true,
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

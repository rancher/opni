package task

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

func EncodeMetadata(input any) *structpb.Struct {
	if input == nil {
		return nil
	}
	if spb, ok := input.(*structpb.Struct); ok {
		return spb
	}
	m := make(map[string]any)

	jsonData, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal metadata of type %T to json: %v", input, err))
	}

	if err := json.Unmarshal(jsonData, &m); err != nil {
		panic(fmt.Sprintf("failed to unmarshal json data to map: %v", err))
	}

	s, err := structpb.NewStruct(m)
	if err != nil {
		panic("unsupported metadata contents: " + err.Error())
	}
	return s
}

func DecodeMetadata(input *structpb.Struct, output any) {
	if input == nil {
		return
	}
	jsonData, err := input.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("failed to marshal *structpb.Struct to json: %v", err))
	}
	if err := json.Unmarshal(jsonData, output); err != nil {
		panic(fmt.Sprintf("failed to unmarshal json data to type %T: %v", output, err))
	}
}

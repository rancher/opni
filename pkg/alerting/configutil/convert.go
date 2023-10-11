package configutil

import (
	"bytes"

	amcfg "github.com/prometheus/alertmanager/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	yamlv2 "gopkg.in/yaml.v2"
)

func MarshalConfig(config *amcfg.Config) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := NewSecretOverrideEncoder(buf)
	if err := encoder.Encode(config); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func MarshalReceiver(config *amcfg.Receiver) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := NewSecretOverrideEncoder(buf)
	if err := encoder.Encode(config); err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func LoadFromAPI[T any](dest T, src proto.Message) error {
	if src == nil {
		return nil
	}
	jsonData, err := protojson.MarshalOptions{
		UseProtoNames: true,
	}.Marshal(src)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to marshal config for api type %s: %v", proto.MessageName(src), err)
	}
	if err := yamlv2.Unmarshal(jsonData, dest); err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to load config for api type %s into %T: %v", proto.MessageName(src), dest, err)
	}
	return nil
}

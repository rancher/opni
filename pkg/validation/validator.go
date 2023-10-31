package validation

import (
	"github.com/bufbuild/protovalidate-go"
	"google.golang.org/protobuf/proto"
)

func NewValidator[T proto.Message](options ...protovalidate.ValidatorOption) (*protovalidate.Validator, error) {
	var t T
	md := t.ProtoReflect().Descriptor()
	if md.IsPlaceholder() {
		panic("link error: no message descriptor available for " + md.FullName())
	}
	parent := md.ParentFile()
	if parent.IsPlaceholder() {
		panic("link error: no file descriptor available for " + parent.Path())
	}

	v, err := protovalidate.New(append(options,
		protovalidate.WithDisableLazy(true),
		protovalidate.WithDescriptors(md),
	)...)
	if err != nil {
		return nil, err
	}
	return v, nil
}

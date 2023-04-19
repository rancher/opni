package util

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/zeebo/xxh3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ServicePackInterface interface {
	Unpack() (*grpc.ServiceDesc, any)
}

type ServicePack[T any] struct {
	desc *grpc.ServiceDesc
	impl T
}

func (s ServicePack[T]) Unpack() (*grpc.ServiceDesc, any) {
	return s.desc, s.impl
}

func PackService[T any](desc *grpc.ServiceDesc, impl T) ServicePack[T] {
	return ServicePack[T]{
		desc: desc,
		impl: impl,
	}
}

func StatusError(code codes.Code) error {
	return status.Error(code, code.String())
}

// Like status.Code(), but supports wrapped errors.
func StatusCode(err error) codes.Code {
	var grpcStatus interface{ GRPCStatus() *status.Status }
	code := codes.Unknown
	if errors.As(err, &grpcStatus) {
		code = grpcStatus.GRPCStatus().Code()
	}
	return code
}

func HashStrings(strings []string) string {
	var buf bytes.Buffer
	for _, s := range strings {
		buf.WriteString(fmt.Sprintf("%s-", s))
	}
	return fmt.Sprintf("%d", HashString(buf.String()))
}

func HashString(s string) uint64 {
	return xxh3.HashString(s)
}

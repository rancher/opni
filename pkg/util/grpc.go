package util

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/zeebo/xxh3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

type ServerStreamWithContext struct {
	Stream grpc.ServerStream
	Ctx    context.Context
}

var _ grpc.ServerStream = (*ServerStreamWithContext)(nil)

func (s *ServerStreamWithContext) SetHeader(md metadata.MD) error {
	return s.Stream.SetHeader(md)
}

func (s *ServerStreamWithContext) SendHeader(md metadata.MD) error {
	return s.Stream.SendHeader(md)
}

func (s *ServerStreamWithContext) SetTrailer(md metadata.MD) {
	s.Stream.SetTrailer(md)
}

func (s *ServerStreamWithContext) Context() context.Context {
	return s.Ctx
}

func (s *ServerStreamWithContext) SendMsg(m interface{}) error {
	return s.Stream.SendMsg(m)
}

func (s *ServerStreamWithContext) RecvMsg(m interface{}) error {
	return s.Stream.RecvMsg(m)
}

type ClientStreamWithContext struct {
	Stream grpc.ClientStream
	Ctx    context.Context
}

var _ grpc.ClientStream = (*ClientStreamWithContext)(nil)

func (s *ClientStreamWithContext) Header() (metadata.MD, error) {
	return s.Stream.Header()
}

func (s *ClientStreamWithContext) Trailer() metadata.MD {
	return s.Stream.Trailer()
}

func (s *ClientStreamWithContext) CloseSend() error {
	return s.Stream.CloseSend()
}

func (s *ClientStreamWithContext) Context() context.Context {
	return s.Ctx
}

func (s *ClientStreamWithContext) SendMsg(m interface{}) error {
	return s.Stream.SendMsg(m)
}

func (s *ClientStreamWithContext) RecvMsg(m interface{}) error {
	return s.Stream.RecvMsg(m)
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

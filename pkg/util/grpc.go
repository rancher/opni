package util

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

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

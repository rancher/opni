package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/grpc"
)

type Protocol uint32

const (
	ProtocolHTTP Protocol = 1 << iota
	ProtocolUnaryGRPC
	ProtocolStreamGRPC
)

type Middleware any

func SupportedProtocols(mw Middleware) Protocol {
	var p Protocol
	if _, ok := mw.(HTTPMiddleware); ok {
		p |= ProtocolHTTP
	}
	if _, ok := mw.(UnaryGRPCMiddleware); ok {
		p |= ProtocolUnaryGRPC
	}
	if _, ok := mw.(StreamGRPCMiddleware); ok {
		p |= ProtocolStreamGRPC
	}
	return p
}

type HTTPMiddleware interface {
	Handle(*fiber.Ctx) error
}

type UnaryGRPCMiddleware interface {
	UnaryServerInterceptor() grpc.UnaryClientInterceptor
}

type StreamGRPCMiddleware interface {
	StreamServerInterceptor() grpc.StreamServerInterceptor
}

var (
	ErrInvalidMiddlewareName   = errors.New("invalid or empty auth middleware name")
	ErrMiddlewareAlreadyExists = errors.New("auth middleware already exists")
	ErrNilMiddleware           = errors.New("auth middleware is nil")
	ErrMiddlewareNotFound      = errors.New("auth middleware not found")
)

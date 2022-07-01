package test

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type AuthStrategy string

const (
	AuthStrategyDenyAll            AuthStrategy = "deny-all"
	AuthStrategyUserIDInAuthHeader AuthStrategy = "user-id-in-auth-header"
)

type TestAuthMiddleware struct {
	Strategy AuthStrategy
}

func (m *TestAuthMiddleware) Handle(c *gin.Context) {
	switch m.Strategy {
	case AuthStrategyDenyAll:
		c.Status(http.StatusUnauthorized)
	case AuthStrategyUserIDInAuthHeader:
		userId := c.GetHeader("Authorization")
		if userId == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Header("Authorization", "")
		c.Set(rbac.UserIDKey, userId)
	default:
		panic("unknown auth strategy")
	}
}

func (m *TestAuthMiddleware) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return grpc.Errorf(codes.InvalidArgument, "no metadata in context")
		}
		authHeader := md.Get(auth.AuthorizationKey)
		if len(authHeader) > 0 && authHeader[0] == "" {
			return grpc.Errorf(codes.InvalidArgument, "authorization header required")
		}
		userId := authHeader[0]
		ss = &util.ServerStreamWithContext{
			Stream: ss,
			Ctx:    metadata.NewIncomingContext(ss.Context(), metadata.New(map[string]string{auth.AuthorizationKey: userId})),
		}
		return handler(srv, ss)
	}
}

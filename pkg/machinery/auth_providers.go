package machinery

import (
	"context"
	"fmt"

	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/noauth"
	"github.com/rancher/opni-monitoring/pkg/auth/openid"
	"github.com/rancher/opni-monitoring/pkg/auth/test"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
)

func LoadAuthProviders(ctx context.Context, objects meta.ObjectList) {
	objects.Visit(
		func(ap *v1beta1.AuthProvider) {
			switch ap.Spec.Type {
			case v1beta1.AuthProviderOpenID:
				mw, err := openid.New(ctx, ap.Spec)
				if err != nil {
					panic(fmt.Errorf("failed to create OpenID auth provider: %w", err))
				}
				if err := auth.RegisterMiddleware(ap.GetName(), mw); err != nil {
					panic(fmt.Errorf("failed to register OpenID auth provider: %w", err))
				}
			case v1beta1.AuthProviderNoAuth:
				mw, err := noauth.New(ctx, ap.Spec)
				if err != nil {
					panic(fmt.Errorf("failed to create noauth auth provider: %w", err))
				}
				if err := auth.RegisterMiddleware(ap.GetName(), mw); err != nil {
					panic(fmt.Errorf("failed to register noauth auth provider: %w", err))
				}
			case "test":
				auth.RegisterMiddleware("test", &test.TestAuthMiddleware{
					Strategy: test.AuthStrategyUserIDInAuthHeader,
				})
			default:
				panic(fmt.Errorf("unsupported auth provider type: %s", ap.Spec.Type))
			}
		},
	)
}

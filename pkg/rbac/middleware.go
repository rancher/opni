package rbac

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"go.uber.org/zap"
)

type middleware struct {
	MiddlewareConfig
}

func (m *middleware) Handle(c *gin.Context) {
	userID, ok := AuthorizedUserID(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	roleList, err := m.fetchRoles(userID)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	headers, err := m.Provider.AccessHeader(context.Background(), roleList)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if len(headers) == 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	for headerKey, list := range headers {
		ids := make([]string, len(list.Items))
		for i, cluster := range list.Items {
			ids[i] = cluster.Id
		}
		c.Request.Header.Set(m.Codec.Key(), m.Codec.Encode(ids))
		c.Set(headerKey, ids)
	}
}

func (m *middleware) fetchRoles(userID string) (*corev1.ReferenceList, error) {
	bindings, err := m.Store.ListRoleBindings(context.Background())
	if err != nil {
		return nil, err
	}
	roleList := &corev1.ReferenceList{}
	for _, binding := range bindings.GetItems() {
		if binding.GetSubject() == userID && binding.GetMetadata().GetCapability() == m.Capability {
			if taints := binding.Taints; len(taints) > 0 {
				m.Logger.With(
					"roleBinding", binding.Id,
					"role", binding.Id,
					"taints", binding.Taints,
				).Warn("skipping tained rolebinding")
				continue
			}
			for _, roleId := range binding.GetRoleIds() {
				roleList.Items = append(roleList.Items, &corev1.Reference{
					Id: roleId,
				})
			}
		}
	}
	return roleList, nil
}

type MiddlewareConfig struct {
	Provider
	Codec      HeaderCodec
	Store      storage.RoleBindingStore
	Capability string
	Logger     *zap.SugaredLogger
}

func NewMiddleware(config MiddlewareConfig) gin.HandlerFunc {
	mw := &middleware{
		MiddlewareConfig: config,
	}
	return mw.Handle
}

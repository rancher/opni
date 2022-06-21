package rbac

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

type middleware struct {
	provider Provider
	codec    HeaderCodec
}

const (
	AuthorizedClusterIDsKey = "authorized_cluster_ids"
)

func (m *middleware) Handle(c *gin.Context) {
	userID, ok := AuthorizedUserID(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	clusters, err := m.provider.SubjectAccess(context.Background(), &corev1.SubjectAccessRequest{
		Subject: userID,
	})
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if len(clusters.Items) == 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	ids := make([]string, len(clusters.Items))
	for i, cluster := range clusters.Items {
		ids[i] = cluster.Id
	}
	c.Request.Header.Set(m.codec.Key(), m.codec.Encode(ids))
	c.Set(AuthorizedClusterIDsKey, ids)
}

func NewMiddleware(provider Provider, codec HeaderCodec) gin.HandlerFunc {
	mw := &middleware{
		provider: provider,
		codec:    codec,
	}
	return mw.Handle
}

func AuthorizedClusterIDs(c *gin.Context) []string {
	value, ok := c.Get(AuthorizedClusterIDsKey)
	if !ok {
		return nil
	}
	return value.([]string)
}

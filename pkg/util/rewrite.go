package util

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// PathRewriter returns a gin middleware to remove a prefix from the URL request path
func PathRewriter(prefix string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		newPath := strings.TrimPrefix(ctx.Request.URL.Path, prefix)
		ctx.Request.URL.Path = newPath
	}
}

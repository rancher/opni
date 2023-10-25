package util

import (
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/plugins/metrics/pkg/constants"
)

func AuthorizedClusterIDs(c *gin.Context) []string {
	value, ok := c.Get(constants.AuthorizedClusterIDsKey)
	if !ok {
		return nil
	}
	return value.([]string)
}

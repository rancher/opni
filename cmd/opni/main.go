package main

import (
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/opni"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	opni.Execute()
}

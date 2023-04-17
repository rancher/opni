package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type GinHandler struct {
	ContentType string
	Route       string
	gin.HandlerFunc
}

func RegisterAdditionalHandlers(router *gin.Engine, handlers []GinHandler) {
	for _, handler := range handlers {
		switch handler.ContentType {
		case http.MethodGet:
			router.GET(handler.Route, handler.HandlerFunc)
		case http.MethodPost:
			router.POST(handler.Route, handler.HandlerFunc)
		case http.MethodDelete:
			router.DELETE(handler.Route, handler.HandlerFunc)
		case http.MethodHead:
			router.HEAD(handler.Route, handler.HandlerFunc)
		case http.MethodPut:
			router.PUT(handler.Route, handler.HandlerFunc)
		case http.MethodPatch:
			router.PATCH(handler.Route, handler.HandlerFunc)
		default:
			panic("unhandled dynamic content type when registering agent handlers")
		}
	}
}

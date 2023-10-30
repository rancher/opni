package router

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	proxyv1 "github.com/rancher/opni/pkg/apis/proxy/v1"
	"github.com/rancher/opni/pkg/proxy/backend"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	pathParam = "proxyPath"
)

type Router interface {
	SetRoutes(*gin.Engine)
}

type RouterConfig struct {
	Store  storage.RoleBindingStore
	Logger *slog.Logger
	Client proxyv1.RegisterProxyClient
}

func NewRouter(config RouterConfig) (Router, error) {
	backend, err := backend.NewBackend(config.Logger.WithGroup("backend"), config.Client)
	if err != nil {
		return nil, err
	}

	endpoint, err := config.Client.Endpoint(context.TODO(), &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	path := endpoint.GetPath()
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &implRouter{
		backend: backend,
		store:   config.Store,
		path:    path,
		logger:  config.Logger,
	}, nil
}

type implRouter struct {
	backend backend.Backend
	store   storage.RoleBindingStore
	path    string
	logger  *slog.Logger
}

func (r *implRouter) handle(c *gin.Context) {
	userID, ok := rbac.AuthorizedUserID(c)
	var roleList *corev1.ReferenceList
	var err error
	if ok {
		roleList, err = r.fetchRoles(userID)
		if err != nil {
			r.logger.With("error", err).Error("failed to fetch roles")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	}
	path, ok := c.Params.Get(pathParam)
	if !ok {
		path = ""
	}
	rewrite, err := r.backend.RewriteProxyRequest(path, roleList)
	if err != nil {
		r.logger.With("error", err).Error("failed to get rewrite function")
		c.AbortWithStatus(http.StatusInternalServerError)
	}
	proxy := httputil.ReverseProxy{
		Rewrite: rewrite,
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (r *implRouter) fetchRoles(userID string) (*corev1.ReferenceList, error) {
	bindings, err := r.store.ListRoleBindings(context.Background())
	if err != nil {
		return nil, err
	}
	roleList := &corev1.ReferenceList{}
	for _, binding := range bindings.GetItems() {
		if slices.Contains(binding.GetSubjects(), userID) {
			roleList.Items = append(roleList.Items, &corev1.Reference{
				Id: binding.RoleId,
			})
		}
	}
	return roleList, nil
}

func (r *implRouter) SetRoutes(router *gin.Engine) {
	router.Any(fmt.Sprintf("%s/*%s", r.path, pathParam), r.handle)
}

package logging

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/b2mac"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ClusterIDHeader = "OpniClusterID"
)

type OpensearchDetailsResponse struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ExternalURL string `json:"externalURL"`
}

func (p *Plugin) ConfigureRoutes(router *gin.Engine) {
	router.Use(logger.GinLogger(p.logger), gin.Recovery())

	storageBackend := p.storageBackend.Get()
	clusterMiddleware, err := cluster.New(p.ctx, storageBackend, ClusterIDHeader)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to set up cluster middleware")
		os.Exit(1)
	}

	v1 := router.Group("/logging/v1", clusterMiddleware.Handle)
	v1.GET("cluster", p.handleGetOpensearchDetails)
}

func (p *Plugin) handleGetOpensearchDetails(c *gin.Context) {
	lg := p.logger

	// Fetch the cluster ID which is set by the middleware
	id := cluster.AuthorizedID(c)

	// Get the external URL
	binding := &opniv1beta2.MulticlusterRoleBinding{}
	if err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      OpensearchBindingName,
		Namespace: p.storageNamespace,
	}, binding); err != nil {
		lg.Error("error fetching opensearch details", "err", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	// Get the credentials
	labels := map[string]string{
		resources.OpniClusterID: id,
	}
	secrets := &corev1.SecretList{}
	if err := p.k8sClient.List(p.ctx, secrets, client.InNamespace(p.storageNamespace), client.MatchingLabels(labels)); err != nil {
		lg.Error("error fetching opensearch details", "err", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	if len(secrets.Items) != 1 {
		lg.Error("failed to list creds")
		c.Status(http.StatusInternalServerError)
		return
	}

	// Return details
	response := OpensearchDetailsResponse{
		Username:    secrets.Items[0].Name,
		Password:    string(secrets.Items[0].Data["password"]),
		ExternalURL: binding.Spec.OpensearchExternalURL,
	}
	responseData, err := json.Marshal(response)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	sharedKeys := cluster.AuthorizedKeys(c)
	header, err := b2mac.NewEncodedHeader([]byte(id), responseData, sharedKeys.ServerKey)
	if err != nil {
		lg.Error("error generating response auth header", "err", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Authorization", header)
	c.Data(http.StatusOK, "application/json", responseData)
}

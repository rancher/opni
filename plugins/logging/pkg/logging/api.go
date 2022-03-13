package logging

import (
	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth/cluster"
	opniv2beta1 "github.com/rancher/opni/apis/v2beta1"
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

func (p *Plugin) ConfigureRoutes(app *fiber.App) {
	storageBackend := p.storageBackend.Get()
	clusterMiddleware := cluster.New(storageBackend, ClusterIDHeader)

	v1 := app.Group("/logging/v1", clusterMiddleware.Handle)
	v1.Get("cluster", p.handleGetOpensearchDetails)
}

func (p *Plugin) handleGetOpensearchDetails(c *fiber.Ctx) error {
	lg := c.Context().Logger()

	// Fetch the cluster ID
	headers := c.GetReqHeaders()
	id, ok := headers[ClusterIDHeader]
	if !ok {
		return ErrClusterIDMissing
	}

	// Get the external URL
	binding := &opniv2beta1.MulticlusterRoleBinding{}
	if err := p.k8sClient.Get(p.ctx, types.NamespacedName{
		Name:      OpensearchBindingName,
		Namespace: p.storageNamespace,
	}, binding); err != nil {
		lg.Printf("error fetching opensearch details: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Get the credentials
	labels := map[string]string{
		resources.OpniClusterID: id,
	}
	secrets := &corev1.SecretList{}
	if err := p.k8sClient.List(p.ctx, secrets, client.InNamespace(p.storageNamespace), client.MatchingLabels(labels)); err != nil {
		lg.Printf("error fetching opensearch details: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	if len(secrets.Items) != 1 {
		lg.Printf("failed to list creds")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Return details
	return c.Status(fiber.StatusOK).JSON(OpensearchDetailsResponse{
		Username:    secrets.Items[0].Name,
		Password:    string(secrets.Items[0].Data["password"]),
		ExternalURL: binding.Spec.OpensearchExternalURL,
	})

}

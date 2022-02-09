package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CredentialsBody struct {
	ID string `json:"id"`
}

type ClusterOp struct {
	Name string `json:"name"`
}

type CreateResponse struct {
	ID string `json:"id"`
}

const (
	DefaultSystemNamespace = "opni-cluster-system"
	LoggingClusterName     = "opni"

	RegisteredAnnotation = "opni.io/cluster-registered"
)

var (
	k8sClient client.Client
)

func SetupAPI(
	ctx context.Context,
	name string,
	namespace string,
	c client.Client,
) error {
	k8sClient = c

	err := k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultSystemNamespace,
		},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	loggingCluster := &v2beta1.LoggingCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LoggingClusterName,
			Namespace: DefaultSystemNamespace,
		},
		Spec: v2beta1.LoggingClusterSpec{
			OpensearchCluster: func() *opensearchv1beta1.OpensearchClusterRef {
				if name == "default" {
					return nil
				}
				var osNamespace string

				if namespace == "default" {
					osNamespace = DefaultSystemNamespace
				} else {
					osNamespace = namespace
				}

				return &opensearchv1beta1.OpensearchClusterRef{
					Name:      name,
					Namespace: osNamespace,
				}
			}(),
		},
	}

	err = util.CreateOrUpdate(ctx, c, loggingCluster, schema.GroupVersionKind{
		Group:   "opni.io",
		Kind:    "LoggingCluster",
		Version: "v2beta1",
	})

	return err
}

// postCreate will create a cluster object in the local k8s cluster
func PutCluster(c *gin.Context) {
	jsonBody := &ClusterOp{}
	if c.Request.ContentLength != 0 {
		err := c.BindJSON(jsonBody)
		if err != nil {
			c.String(http.StatusBadRequest, "could not parse body")
			return
		}
	}

	// grab the name from the context.
	name := c.Query("name")
	if jsonBody.Name != "" {
		name = jsonBody.Name
	}

	if name == "" {
		c.String(http.StatusBadRequest, "name must be provided")
		return
	}

	id, err := createClusterObject(c, name)
	if k8serrors.IsAlreadyExists(err) {
		c.String(http.StatusBadRequest, "cluster name is already registered")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed to create object")
		return
	}

	token := util.GenerateRandomString(20)
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cluster-token", name),
			Namespace: DefaultSystemNamespace,
		},
		Data: map[string][]byte{
			"token": token,
		},
	}
	err = k8sClient.Create(c, tokenSecret)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to create token")
		return
	}

	c.JSON(http.StatusCreated, CreateResponse{ID: id})
}

func DeleteCluster(c *gin.Context) {
	jsonBody := &ClusterOp{}
	if c.Request.ContentLength != 0 {
		err := c.BindJSON(jsonBody)
		if err != nil {
			c.String(http.StatusBadRequest, "could not parse body")
			return
		}
	}

	// grab the name from the context.
	name := c.Query("name")
	if jsonBody.Name != "" {
		name = jsonBody.Name
	}

	if name == "" {
		c.String(http.StatusBadRequest, "name must be provided")
		return
	}

	cluster := &v2beta1.DownstreamCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: DefaultSystemNamespace,
		},
	}
	err := k8sClient.Delete(c, cluster)
	if err != nil && !k8serrors.IsNotFound(err) {
		c.String(http.StatusInternalServerError, "failed to delete cluster object")
		return
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cluster-token", name),
			Namespace: DefaultSystemNamespace,
		},
	}
	err = k8sClient.Delete(c, secret)
	if err != nil && !k8serrors.IsNotFound(err) {
		c.String(http.StatusInternalServerError, "failed to delete token secret")
		return
	}

	c.String(http.StatusOK, "cluster deleted")
}

func GetClusterCredentials(c *gin.Context) {
	jsonBody := &CredentialsBody{}
	if c.Request.ContentLength != 0 {
		err := c.BindJSON(jsonBody)
		if err != nil {
			c.String(http.StatusBadRequest, "could not parse body")
			return
		}
	}

	// grab the id from the context.
	id := c.Query("id")
	if jsonBody.ID != "" {
		id = jsonBody.ID
	}

	if id == "" {
		c.String(http.StatusBadRequest, "id must be provided")
		return
	}

	labels := map[string]string{
		v2beta1.IDLabel: id,
	}

	clusterList := &v2beta1.DownstreamClusterList{}

	err := k8sClient.List(c, clusterList, client.InNamespace(DefaultSystemNamespace), client.MatchingLabels(labels))
	if err != nil {
		c.String(http.StatusInternalServerError, "could not search for credentials")
		return
	}

	if len(clusterList.Items) == 0 {
		c.Status(http.StatusNotFound)
		return
	}

	if len(clusterList.Items) > 1 {
		c.String(http.StatusInternalServerError, "multiple downstream clusters found")
		return
	}

	cluster := clusterList.Items[0]
	if cluster.Status.IndexUserSecretRef == nil {
		c.String(http.StatusAccepted, "cluster not ready")
		return
	}

	secret := &corev1.Secret{}

	err = k8sClient.Get(c, types.NamespacedName{
		Name:      cluster.Status.IndexUserSecretRef.Name,
		Namespace: DefaultSystemNamespace,
	}, secret)
	if err != nil {
		c.String(http.StatusInternalServerError, "could not search for user secret")
		return
	}

	c.JSON(http.StatusOK, Credentials{
		Username: secret.Name,
		Password: string(secret.Data["password"]),
	})
}

func createClusterObject(ctx context.Context, name string) (retID string, retErr error) {
	id := uuid.New()
	retID = id.String()
	cluster := &v2beta1.DownstreamCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: DefaultSystemNamespace,
			Labels: map[string]string{
				v2beta1.IDLabel: retID,
			},
		},
		Spec: v2beta1.DownstreamClusterSpec{
			LoggingClusterRef: &corev1.LocalObjectReference{
				Name: LoggingClusterName,
			},
		},
	}

	retErr = k8sClient.Create(ctx, cluster)
	return
}

package gateway

import (
	"encoding/json"

	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/keyring/ephemeral"
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) ephemeralKeys() ([]resources.Resource, error) {
	localAgentKey := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-local-agent-key",
			Namespace: r.gw.Namespace,
		},
		Type:      corev1.SecretTypeOpaque,
		Immutable: lo.ToPtr(true),
	}
	err := r.client.Get(r.ctx, client.ObjectKeyFromObject(localAgentKey), localAgentKey)
	if k8serrors.IsNotFound(err) {
		key := ephemeral.NewKey(ephemeral.Authentication, map[string]string{
			session.AttributeLabelKey: "local",
		})
		keyData, _ := json.Marshal(key)
		localAgentKey.Data = map[string][]byte{
			"session-attribute.json": keyData,
		}
	} else if err != nil {
		return nil, err
	}
	ctrl.SetControllerReference(r.gw, localAgentKey, r.client.Scheme())
	return []resources.Resource{resources.Present(localAgentKey)}, nil
}

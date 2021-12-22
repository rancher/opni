package bootstrap

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
)

const defaultSecretName = "opni-gateway-bootstrap-tokens"

type InClusterSecretStoreOptions struct {
	namespace  string
	secretName string
}

type InClusterSecretStoreOption func(*InClusterSecretStoreOptions)

func (o *InClusterSecretStoreOptions) Apply(opts ...InClusterSecretStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(ns string) InClusterSecretStoreOption {
	return func(o *InClusterSecretStoreOptions) {
		o.namespace = ns
	}
}

func WithSecretName(name string) InClusterSecretStoreOption {
	return func(o *InClusterSecretStoreOptions) {
		o.secretName = name
	}
}

type InClusterSecretStore struct {
	ctx       context.Context
	clientset *kubernetes.Clientset
	cache     tokenCache
	ns        string
}

var _ TokenStore = (*InClusterSecretStore)(nil)

func NewInClusterSecretStore(
	ctx context.Context,
	options ...InClusterSecretStoreOption,
) (TokenStore, error) {
	opts := InClusterSecretStoreOptions{
		namespace:  os.Getenv("POD_NAMESPACE"),
		secretName: defaultSecretName,
	}
	opts.Apply(options...)
	if opts.namespace == "" {
		return nil, fmt.Errorf("namespace is not set")
	}
	if opts.secretName == "" {
		return nil, fmt.Errorf("secret name is not set")
	}
	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	informer := informers.NewFilteredSharedInformerFactory(
		clientset, 0, opts.namespace, func(lo *metav1.ListOptions) {
			lo.FieldSelector = "metadata.name=" + opts.secretName
		}).Core().V1().Secrets().Informer()
	i := &InClusterSecretStore{
		ctx:       ctx,
		clientset: clientset,
		ns:        opts.namespace,
	}
	informer.AddEventHandler(i)
	go informer.Run(ctx.Done())
	// this will block until the informer has synced, or the context is canceled
	fmt.Println("Waiting for caches to sync")
	if !cache.WaitForNamedCacheSync("bootstrap tokens", ctx.Done(), informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return nil, err
	}
	if err := i.createOrSyncSecret(ctx); err != nil {
		return nil, err
	}
	fmt.Println("Caches synced successfully")
	return i, nil
}

func (i *InClusterSecretStore) OnAdd(obj interface{}) {
	fmt.Println("Token secret created")
	sec := obj.(*corev1.Secret).DeepCopy()
	i.cache.Lock()
	defer i.cache.Unlock()
	for k, v := range sec.Data {
		i.cache.data[k] = string(v)
	}
}

func (i *InClusterSecretStore) OnUpdate(oldObj, newObj interface{}) {
	fmt.Println("Token secret updated")
	sec := newObj.(*corev1.Secret).DeepCopy()
	i.cache.Lock()
	defer i.cache.Unlock()
	for k, v := range sec.Data {
		i.cache.data[k] = string(v)
	}
}

func (i *InClusterSecretStore) OnDelete(obj interface{}) {
	fmt.Println("Token secret deleted, will recreate")
	i.cache.Lock()
	i.cache.data = map[string]string{}
	i.cache.Unlock()
	if err := i.createOrSyncSecret(i.ctx); err != nil {
		runtime.HandleError(err)
	}
}

func (i *InClusterSecretStore) CreateToken(ctx context.Context) (*Token, error) {
	token := NewToken()
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		sec, err := i.getSecret()
		if err != nil {
			return err
		}
		sec.StringData[token.HexID()] = token.EncodeHex()
		_, err = i.clientset.CoreV1().Secrets(i.ns).Update(ctx, sec, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (i *InClusterSecretStore) DeleteToken(ctx context.Context, token string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		sec, err := i.getSecret()
		if err != nil {
			return err
		}
		if _, ok := sec.Data[token]; !ok {
			return nil
		}
		delete(sec.Data, token)
		_, err = i.clientset.CoreV1().Secrets(i.ns).Update(ctx, sec, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return err
	}
	return nil
}

func (i *InClusterSecretStore) TokenExists(ctx context.Context, token string) (bool, error) {
	i.cache.Lock()
	defer i.cache.Unlock()
	_, ok := i.cache.data[token]
	return ok, nil
}

func (i *InClusterSecretStore) GetToken(ctx context.Context, tokenID string) (*Token, error) {
	i.cache.Lock()
	defer i.cache.Unlock()
	if secret, ok := i.cache.data[tokenID]; ok {
		return DecodeHexToken(secret)
	}
	return nil, nil
}

func (i *InClusterSecretStore) ListTokens(ctx context.Context) ([]string, error) {
	i.cache.Lock()
	defer i.cache.Unlock()
	var tokens []string
	for id := range i.cache.data {
		tokens = append(tokens, id)
	}
	return tokens, nil
}

func (i *InClusterSecretStore) getSecret() (*corev1.Secret, error) {
	sec, err := i.clientset.CoreV1().
		Secrets(i.ns).
		Get(context.Background(), defaultSecretName, metav1.GetOptions{})
	return sec.DeepCopy(), err
}

func (i *InClusterSecretStore) createOrSyncSecret(ctx context.Context) error {
	i.cache.Lock()
	defer i.cache.Unlock()
	if sec, err := i.getSecret(); err == nil {
		for k := range sec.Data {
			i.cache.data[k] = string(sec.Data[k])
		}
		return nil
	}
	_, err := i.clientset.CoreV1().Secrets(i.ns).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultSecretName,
		},
		Data: map[string][]byte{},
	}, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

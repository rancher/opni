package modeltraining

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	opensearch "github.com/opensearch-project/opensearch-go"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
)

func newNatsConnection() (*nats.Conn, error) {
	natsURL := os.Getenv("NATS_SERVER_URL")
	natsSeedPath := os.Getenv("NKEY_SEED_FILENAME")

	opt, err := nats.NkeyOptionFromSeed(natsSeedPath)
	if err != nil {
		return nil, err
	}

	retryBackoff := backoff.NewExponentialBackOff()
	return nats.Connect(
		natsURL,
		opt,
		nats.MaxReconnects(-1),
		nats.CustomReconnectDelay(
			func(i int) time.Duration {
				if i == 1 {
					retryBackoff.Reset()
				}
				return retryBackoff.NextBackOff()
			},
		),
	)
}

func (s *ModelTrainingPlugin) newOpensearchConnection() (*opensearch.Client, error) {
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = "opni-cluster-system"
	}
	esEndpoint := fmt.Sprintf("https://opni-opensearch-svc.%s.svc:9200", namespace)
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(s.ctx)
	cluster := &opsterv1.OpenSearchCluster{}
FETCH:
	for {
		select {
		case <-b.Done():
			s.Logger.Warn("plugin context cancelled before Opensearch object created")
		case <-b.Next():
			err := s.k8sClient.Get().Get(s.ctx, types.NamespacedName{
				Name:      "opni",
				Namespace: namespace,
			}, cluster)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					s.Logger.Info("waiting for k8s object")
					continue
				}
				s.Logger.Errorf("failed to check k8s object: %v", err)
				continue
			}
			break FETCH
		}
	}
	esUsername, esPassword, err := helpers.UsernameAndPassword(s.ctx, s.k8sClient.Get(), cluster)
	if err != nil {
		return nil, err
	}
	osClient, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{esEndpoint},
		Username:  esUsername,
		Password:  esPassword,
	})
	return osClient, err
}

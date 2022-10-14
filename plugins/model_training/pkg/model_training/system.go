package model_training

import (
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/nats-io/nats.go"
	opensearch "github.com/opensearch-project/opensearch-go"
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

func newOpensearchConnection() (*opensearch.Client, error) {
	ES_ENDPOINT := "https://opni-opensearch-svc.opni-cluster-system.svc:9200"
	ES_USERNAME := "admin"
	ES_PASSWORD := "admin"
	return opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{ES_ENDPOINT},
		Username:  ES_USERNAME,
		Password:  ES_PASSWORD,
	})
}

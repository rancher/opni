//go:build !unix

package clients

import "errors"

type ConnStats struct{}

func (*gatewayClient) QueryConnStats() (ConnStats, error) {
	return ConnStats{}, errors.New("unsupported")
}

package test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/goombaio/namegenerator"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func RandomName(seed int64) string {
	nameGenerator := namegenerator.NewNameGenerator(seed)
	return nameGenerator.Generate()
}

// StandaloneHttpRequest is a helper function to make a http request to a given url.
// returns status code, body and any errors
func StandaloneHttpRequest(
	method string,
	url string,
	requestBody []byte,
	values url.Values,
	tls *tls.Config,
) (code int, body []byte, err error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tls,
		},
	}
	req, err := http.NewRequest(method, url, func() io.Reader {
		if requestBody != nil {
			return bytes.NewReader(requestBody)
		}
		return nil
	}())
	if err != nil {
		return 0, nil, err
	}
	if values != nil && len(values) > 0 {
		req.URL.RawQuery = values.Encode()
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func ExpectRuleGroupToExist(
	client cortexadmin.CortexAdminClient,
	ctx context.Context,
	tenant string,
	groupName string,
	pollInterval time.Duration,
	maxTimeout time.Duration,
) {
	Eventually(func() error {
		_, err := client.GetRule(ctx, &cortexadmin.RuleRequest{
			ClusterId: tenant,
			GroupName: groupName,
		})
		if err != nil {
			return err
		}
		if status.Code(err) == codes.NotFound {
			return fmt.Errorf("rule group %s not found", groupName)
		}
		return nil
	}, maxTimeout, pollInterval).Should(Succeed())
}

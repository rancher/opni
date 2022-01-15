package integration_test

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Simple Test", Ordered, func() {
	var environment *test.Environment
	BeforeAll(func() {
		fmt.Println("Starting test environment")
		environment = &test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(environment.Start()).To(Succeed())
	})

	AfterAll(func() {
		fmt.Println("Stopping test environment")
		Expect(environment.Stop()).To(Succeed())
	})

	Specify("it should start the gateway", func() {
		gc := environment.GatewayConfig().Spec.ListenAddress
		Eventually(func() int {
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/healthz", gc), nil)
			client := http.Client{
				Transport: &http.Transport{
					TLSClientConfig: environment.GatewayTLSConfig(),
				},
			}
			resp, err := client.Do(req)
			if err != nil {
				return -1
			}
			return resp.StatusCode
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(http.StatusOK))
	})
	Specify("cortex should become ready", func() {
		gc := environment.GatewayConfig().Spec.ListenAddress
		Eventually(func() int {
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/ready", gc), nil)
			client := http.Client{
				Transport: &http.Transport{
					TLSClientConfig: environment.GatewayTLSConfig(),
				},
			}
			resp, err := client.Do(req)
			if err != nil {
				return -1
			}
			return resp.StatusCode
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(http.StatusOK))
	})
})

package util_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/test/testgrpc"
	"github.com/rancher/opni/pkg/util"
)

var _ = BuildHttpTransportCaching(
	util.NewInternalHttpCacheTransport(
		caching.NewInMemoryHttpTtlCache("50Mi", time.Second*1),
	),
)

func BuildHttpTransportCaching(
	t util.HttpCachingTransport,
) bool {
	return Describe("Http util test suites", Ordered, Label("unit"), func() {
		var serverPort int
		var cachingClient *http.Client
		var defaultClient *http.Client
		doIncrement := func(objectId string) (*http.Response, error) {
			var url string
			if objectId == "" {
				url = fmt.Sprintf("http://localhost:%d/increment", serverPort)
			} else {
				url = fmt.Sprintf("http://localhost:%d/increment?id=%s", serverPort, objectId)
			}
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Object-Id", objectId)
			return defaultClient.Do(req)
		}

		getValueWithMaxAgeCache := func(objectId string) (*http.Response, error) {
			var url string
			if objectId == "" {
				url = fmt.Sprintf("http://localhost:%d/value", serverPort)
			} else {
				url = fmt.Sprintf("http://localhost:%d/value?id=%s", serverPort, objectId)
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Object-Id", objectId)
			util.WithHttpMaxAgeCachingHeader(req.Header, time.Second*5)
			return cachingClient.Do(req)
		}

		getValueWithBypassCache := func(objectId string) (*http.Response, error) {
			var url string
			if objectId == "" {
				url = fmt.Sprintf("http://localhost:%d/value", serverPort)
			} else {
				url = fmt.Sprintf("http://localhost:%d/value?id=%s", serverPort, objectId)
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Object-Id", objectId)
			util.WithHttpNoCachingHeader(req.Header)
			return cachingClient.Do(req)
		}

		getValueDefaultTransport := func(objectId string) (*http.Response, error) {
			var url string
			if objectId == "" {
				url = fmt.Sprintf("http://localhost:%d/value", serverPort)
			} else {
				url = fmt.Sprintf("http://localhost:%d/value?id=%s", serverPort, objectId)
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Object-Id", objectId)
			return defaultClient.Do(req)
		}

		getServerCacheValue := func() (*http.Response, error) {
			url := fmt.Sprintf("http://localhost:%d/cache/value", serverPort)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			return cachingClient.Do(req)
		}

		BeforeAll(func() {
			port, err := freeport.GetFreePort()
			Expect(err).To(Succeed())
			serverPort = port

			cachingHttpServer := testgrpc.NewCachingHttpServer(
				serverPort,
			)

			go func() {
				err := cachingHttpServer.ListenAndServe()
				if !errors.Is(err, http.ErrServerClosed) {
					panic(err)
				}
			}()

			DeferCleanup(func() {
				cachingHttpServer.Shutdown(context.TODO())
			})
			defaultClient = http.DefaultClient
			client := http.DefaultClient
			err = t.Use(client)
			Expect(err).To(Succeed())
			Expect(client.Transport).NotTo(BeNil())
			Expect(serverPort).NotTo(BeZero())
			cachingClient = client

		})
		When("We use standardized cache control headers with our http caching transport", func() {
			It("should not replace custom transports", func() {
				Expect(cachingClient.Transport).NotTo(BeNil())
				Expect(defaultClient.Transport).NotTo(BeNil())
			})
			It("should implement the cache-control headers", func() {

				var data testgrpc.ValueResponse
				By("sending a request to increment the value")
				resp, err := doIncrement("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				resp.Body.Close()

				By("fetching the value with a default http.RoundTripper")
				resp, err = getValueDefaultTransport("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(1))
				resp.Body.Close()

				By("fetching the value with a caching http.RoundTripper")
				resp, err = getValueWithMaxAgeCache("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(1))
				resp.Body.Close()

				for i := 0; i < 10; i++ {
					resp, err := doIncrement("")
					Expect(err).To(Succeed())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				}

				By("ensuring subsequent requests through the caching transport are cached")
				resp, err = getValueWithMaxAgeCache("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(1))

				By("ensuring clients using the caching transport can bypass the cache")
				resp, err = getValueWithBypassCache("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(11))

				By("veryfing that the cache will eventually expire")
				Eventually(func() int {
					resp, err := getValueWithMaxAgeCache("")
					if err != nil {
						return -1
					}
					if resp.StatusCode != http.StatusOK {
						return -1
					}
					err = json.NewDecoder(resp.Body).Decode(&data)
					Expect(err).To(Succeed())
					defer resp.Body.Close()
					return data.Value
				}, time.Second*30, time.Second).Should(Equal(11))

			})

			It("should allow servers to tell clients using the cache transport to cache requests", func() {
				By("having the server cache the value")
				var data testgrpc.ValueResponse
				resp, err := getServerCacheValue()
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(11))
				resp.Body.Close()

				for i := 0; i < 5; i++ {
					resp, err := doIncrement("")
					Expect(err).To(Succeed())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				}

				By("ensuring subsequent requests through the caching transport are cached")
				resp, err = getValueWithMaxAgeCache("")
				Expect(err).To(Succeed())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				err = json.NewDecoder(resp.Body).Decode(&data)
				Expect(err).To(Succeed())
				Expect(data.Value).To(Equal(11))

				By("ensuring that the cache set by the server will expire")
				Eventually(func() int {
					resp, err := getValueWithMaxAgeCache("")
					if err != nil {
						return -1
					}
					if resp.StatusCode != http.StatusOK {
						return -1
					}
					err = json.NewDecoder(resp.Body).Decode(&data)
					Expect(err).To(Succeed())
					defer resp.Body.Close()
					return data.Value
				}, time.Second*30, time.Second).Should(Equal(16))
			})
		})
	})
}

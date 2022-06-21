package openid_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/auth/openid"
)

func TestOpenid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Openid Suite")
}

var discovery *testDiscoveryServer

func newRandomRS256Key() jwk.Key {
	var err error
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	key, err := jwk.New(privKey)
	if err != nil {
		panic(err)
	}
	if err := jwk.AssignKeyID(key); err != nil {
		panic(err)
	}
	key.Set(jwk.AlgorithmKey, jwa.RS256)
	return key
}

type testDiscoveryServer struct {
	addr         string
	key          jwk.Key
	wellKnownCfg openid.WellKnownConfiguration
}

func newTestDiscoveryServer(ctx context.Context, portOverride ...int) *testDiscoveryServer {
	key := newRandomRS256Key()
	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())
	if len(portOverride) == 1 {
		port = portOverride[0]
	}
	addr := fmt.Sprintf("localhost:%d", port)
	wellKnownCfg := openid.WellKnownConfiguration{
		Issuer:           "http://" + addr,
		AuthEndpoint:     "http://" + addr + "/authorize",
		TokenEndpoint:    "http://" + addr + "/token",
		UserinfoEndpoint: "http://" + addr + "/userinfo",
		JwksUri:          "http://" + addr + "/jwks",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, err := json.Marshal(wellKnownCfg)
		Expect(err).NotTo(HaveOccurred())
		w.Write(data)
	})
	mux.HandleFunc("/bad-redirect-test", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/.well-known/openid-configuration", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/bad-json-test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"foo\":\"bar}"))
	})
	mux.HandleFunc("/bad-response-code-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/timeout", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sub":"test"}`))
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		set := jwk.NewSet()
		pub, err := key.PublicKey()
		if err != nil {
			panic(err)
		}
		set.Add(pub)
		data, err := json.Marshal(set)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		w.Write(data)
	})
	mux.HandleFunc("/missing-fields-test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		copied := wellKnownCfg
		copied.JwksUri = ""
		data, err := json.Marshal(copied)
		Expect(err).NotTo(HaveOccurred())
		w.Write(data)
	})
	srv := http.Server{
		Addr:    addr,
		Handler: mux,
	}
	go srv.ListenAndServe()
	go func() {
		<-ctx.Done()
		srv.Shutdown(ctx)
	}()
	return &testDiscoveryServer{
		addr:         addr,
		wellKnownCfg: wellKnownCfg,
		key:          key,
	}
}

var _ = BeforeSuite(func() {
	ctx, ca := context.WithCancel(context.Background())
	discovery = newTestDiscoveryServer(ctx)
	DeferCleanup(ca)
})

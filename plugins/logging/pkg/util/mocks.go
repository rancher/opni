package util

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/jarcoal/httpmock"
	"github.com/rancher/opni/pkg/opensearch/certs"
)

const (
	OpensearchURL = "https://mock-opensearch.example.com"
)

var (
	mockCertReader certs.OpensearchCertReader
)

type MockInstallState struct {
	stateMutex       sync.RWMutex
	installStarted   bool
	installCompleted bool
}

func (s *MockInstallState) StartInstall() {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	s.installStarted = true
}

func (s *MockInstallState) CompleteInstall() {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	s.installCompleted = true
}

func (s *MockInstallState) Uninstall() {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()
	s.installCompleted = false
	s.installStarted = false
}

func (s *MockInstallState) IsStarted() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	return s.installStarted
}

func (s *MockInstallState) IsCompleted() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()
	return s.installCompleted
}

func OpensearchMockTransport() http.RoundTripper {
	transport := httpmock.NewMockTransport()
	transport.RegisterNoResponder(httpmock.NewNotFoundResponder(nil))

	statusResp := map[string]string{
		"status": "green",
	}
	transport.RegisterResponder(
		http.MethodGet,
		fmt.Sprintf("%s/_cluster/health", OpensearchURL),
		httpmock.NewJsonResponderOrPanic(200, statusResp),
	)

	transport.RegisterResponder(
		http.MethodPost,
		fmt.Sprintf("=~%s/opni-cluster-metadata/_doc/.+", OpensearchURL),
		httpmock.NewStringResponder(200, ""),
	)

	transport.RegisterResponder(
		http.MethodGet,
		fmt.Sprintf("%s/_plugins/_security/api/internalusers/opni", OpensearchURL),
		httpmock.NewStringResponder(200, ""),
	)

	return transport
}

func GetMockCertReader() certs.OpensearchCertReader {
	return mockCertReader
}

func SetMockCertReader(r certs.OpensearchCertReader) {
	mockCertReader = r
}

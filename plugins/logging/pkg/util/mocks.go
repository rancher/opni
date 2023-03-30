package util

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/jarcoal/httpmock"
)

const (
	OpensearchURL = "https://mock-opensearch.example.com"
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

	return transport
}

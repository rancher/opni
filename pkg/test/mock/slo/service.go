// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/slo/backend/service.go

// Package mock_backend is a generated GoMock package.
package mock_backend

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1 "github.com/rancher/opni/pkg/apis/slo/v1"
)

// MockServiceBackend is a mock of ServiceBackend interface.
type MockServiceBackend struct {
	ctrl     *gomock.Controller
	recorder *MockServiceBackendMockRecorder
}

// MockServiceBackendMockRecorder is the mock recorder for MockServiceBackend.
type MockServiceBackendMockRecorder struct {
	mock *MockServiceBackend
}

// NewMockServiceBackend creates a new mock instance.
func NewMockServiceBackend(ctrl *gomock.Controller) *MockServiceBackend {
	mock := &MockServiceBackend{ctrl: ctrl}
	mock.recorder = &MockServiceBackendMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockServiceBackend) EXPECT() *MockServiceBackendMockRecorder {
	return m.recorder
}

// ListEvents mocks base method.
func (m *MockServiceBackend) ListEvents(ctx context.Context, req *v1.ListEventsRequest) (*v1.EventList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListEvents", ctx, req)
	ret0, _ := ret[0].(*v1.EventList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListEvents indicates an expected call of ListEvents.
func (mr *MockServiceBackendMockRecorder) ListEvents(ctx, req interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListEvents", reflect.TypeOf((*MockServiceBackend)(nil).ListEvents), ctx, req)
}

// ListMetrics mocks base method.
func (m *MockServiceBackend) ListMetrics(ctx context.Context, req *v1.ListMetricsRequest) (*v1.MetricGroupList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListMetrics", ctx, req)
	ret0, _ := ret[0].(*v1.MetricGroupList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListMetrics indicates an expected call of ListMetrics.
func (mr *MockServiceBackendMockRecorder) ListMetrics(ctx, req interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListMetrics", reflect.TypeOf((*MockServiceBackend)(nil).ListMetrics), ctx, req)
}

// ListServices mocks base method.
func (m *MockServiceBackend) ListServices(ctx context.Context, req *v1.ListServicesRequest) (*v1.ServiceList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListServices", ctx, req)
	ret0, _ := ret[0].(*v1.ServiceList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListServices indicates an expected call of ListServices.
func (mr *MockServiceBackendMockRecorder) ListServices(ctx, req interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListServices", reflect.TypeOf((*MockServiceBackend)(nil).ListServices), ctx, req)
}

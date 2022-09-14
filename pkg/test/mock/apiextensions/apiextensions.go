// Code generated by MockGen. DO NOT EDIT.
// Source: pkg/plugins/apis/apiextensions/apiextensions_grpc.pb.go

// Package mock_apiextensions is a generated GoMock package.
package mock_apiextensions

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	apiextensions "github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	grpc "google.golang.org/grpc"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// MockManagementAPIExtensionClient is a mock of ManagementAPIExtensionClient interface.
type MockManagementAPIExtensionClient struct {
	ctrl     *gomock.Controller
	recorder *MockManagementAPIExtensionClientMockRecorder
}

// MockManagementAPIExtensionClientMockRecorder is the mock recorder for MockManagementAPIExtensionClient.
type MockManagementAPIExtensionClientMockRecorder struct {
	mock *MockManagementAPIExtensionClient
}

// NewMockManagementAPIExtensionClient creates a new mock instance.
func NewMockManagementAPIExtensionClient(ctrl *gomock.Controller) *MockManagementAPIExtensionClient {
	mock := &MockManagementAPIExtensionClient{ctrl: ctrl}
	mock.recorder = &MockManagementAPIExtensionClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockManagementAPIExtensionClient) EXPECT() *MockManagementAPIExtensionClientMockRecorder {
	return m.recorder
}

// Descriptors mocks base method.
func (m *MockManagementAPIExtensionClient) Descriptors(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*apiextensions.ServiceDescriptorProtoList, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Descriptors", varargs...)
	ret0, _ := ret[0].(*apiextensions.ServiceDescriptorProtoList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Descriptors indicates an expected call of Descriptors.
func (mr *MockManagementAPIExtensionClientMockRecorder) Descriptors(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Descriptors", reflect.TypeOf((*MockManagementAPIExtensionClient)(nil).Descriptors), varargs...)
}

// MockManagementAPIExtensionServer is a mock of ManagementAPIExtensionServer interface.
type MockManagementAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockManagementAPIExtensionServerMockRecorder
}

// MockManagementAPIExtensionServerMockRecorder is the mock recorder for MockManagementAPIExtensionServer.
type MockManagementAPIExtensionServerMockRecorder struct {
	mock *MockManagementAPIExtensionServer
}

// NewMockManagementAPIExtensionServer creates a new mock instance.
func NewMockManagementAPIExtensionServer(ctrl *gomock.Controller) *MockManagementAPIExtensionServer {
	mock := &MockManagementAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockManagementAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockManagementAPIExtensionServer) EXPECT() *MockManagementAPIExtensionServerMockRecorder {
	return m.recorder
}

// Descriptors mocks base method.
func (m *MockManagementAPIExtensionServer) Descriptors(arg0 context.Context, arg1 *emptypb.Empty) (*apiextensions.ServiceDescriptorProtoList, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Descriptors", arg0, arg1)
	ret0, _ := ret[0].(*apiextensions.ServiceDescriptorProtoList)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Descriptors indicates an expected call of Descriptors.
func (mr *MockManagementAPIExtensionServerMockRecorder) Descriptors(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Descriptors", reflect.TypeOf((*MockManagementAPIExtensionServer)(nil).Descriptors), arg0, arg1)
}

// mustEmbedUnimplementedManagementAPIExtensionServer mocks base method.
func (m *MockManagementAPIExtensionServer) mustEmbedUnimplementedManagementAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedManagementAPIExtensionServer")
}

// mustEmbedUnimplementedManagementAPIExtensionServer indicates an expected call of mustEmbedUnimplementedManagementAPIExtensionServer.
func (mr *MockManagementAPIExtensionServerMockRecorder) mustEmbedUnimplementedManagementAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedManagementAPIExtensionServer", reflect.TypeOf((*MockManagementAPIExtensionServer)(nil).mustEmbedUnimplementedManagementAPIExtensionServer))
}

// MockUnsafeManagementAPIExtensionServer is a mock of UnsafeManagementAPIExtensionServer interface.
type MockUnsafeManagementAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockUnsafeManagementAPIExtensionServerMockRecorder
}

// MockUnsafeManagementAPIExtensionServerMockRecorder is the mock recorder for MockUnsafeManagementAPIExtensionServer.
type MockUnsafeManagementAPIExtensionServerMockRecorder struct {
	mock *MockUnsafeManagementAPIExtensionServer
}

// NewMockUnsafeManagementAPIExtensionServer creates a new mock instance.
func NewMockUnsafeManagementAPIExtensionServer(ctrl *gomock.Controller) *MockUnsafeManagementAPIExtensionServer {
	mock := &MockUnsafeManagementAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockUnsafeManagementAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnsafeManagementAPIExtensionServer) EXPECT() *MockUnsafeManagementAPIExtensionServerMockRecorder {
	return m.recorder
}

// mustEmbedUnimplementedManagementAPIExtensionServer mocks base method.
func (m *MockUnsafeManagementAPIExtensionServer) mustEmbedUnimplementedManagementAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedManagementAPIExtensionServer")
}

// mustEmbedUnimplementedManagementAPIExtensionServer indicates an expected call of mustEmbedUnimplementedManagementAPIExtensionServer.
func (mr *MockUnsafeManagementAPIExtensionServerMockRecorder) mustEmbedUnimplementedManagementAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedManagementAPIExtensionServer", reflect.TypeOf((*MockUnsafeManagementAPIExtensionServer)(nil).mustEmbedUnimplementedManagementAPIExtensionServer))
}

// MockHTTPAPIExtensionClient is a mock of HTTPAPIExtensionClient interface.
type MockHTTPAPIExtensionClient struct {
	ctrl     *gomock.Controller
	recorder *MockHTTPAPIExtensionClientMockRecorder
}

// MockHTTPAPIExtensionClientMockRecorder is the mock recorder for MockHTTPAPIExtensionClient.
type MockHTTPAPIExtensionClientMockRecorder struct {
	mock *MockHTTPAPIExtensionClient
}

// NewMockHTTPAPIExtensionClient creates a new mock instance.
func NewMockHTTPAPIExtensionClient(ctrl *gomock.Controller) *MockHTTPAPIExtensionClient {
	mock := &MockHTTPAPIExtensionClient{ctrl: ctrl}
	mock.recorder = &MockHTTPAPIExtensionClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHTTPAPIExtensionClient) EXPECT() *MockHTTPAPIExtensionClientMockRecorder {
	return m.recorder
}

// Configure mocks base method.
func (m *MockHTTPAPIExtensionClient) Configure(ctx context.Context, in *apiextensions.CertConfig, opts ...grpc.CallOption) (*apiextensions.HTTPAPIExtensionConfig, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Configure", varargs...)
	ret0, _ := ret[0].(*apiextensions.HTTPAPIExtensionConfig)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Configure indicates an expected call of Configure.
func (mr *MockHTTPAPIExtensionClientMockRecorder) Configure(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Configure", reflect.TypeOf((*MockHTTPAPIExtensionClient)(nil).Configure), varargs...)
}

// MockHTTPAPIExtensionServer is a mock of HTTPAPIExtensionServer interface.
type MockHTTPAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockHTTPAPIExtensionServerMockRecorder
}

// MockHTTPAPIExtensionServerMockRecorder is the mock recorder for MockHTTPAPIExtensionServer.
type MockHTTPAPIExtensionServerMockRecorder struct {
	mock *MockHTTPAPIExtensionServer
}

// NewMockHTTPAPIExtensionServer creates a new mock instance.
func NewMockHTTPAPIExtensionServer(ctrl *gomock.Controller) *MockHTTPAPIExtensionServer {
	mock := &MockHTTPAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockHTTPAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHTTPAPIExtensionServer) EXPECT() *MockHTTPAPIExtensionServerMockRecorder {
	return m.recorder
}

// Configure mocks base method.
func (m *MockHTTPAPIExtensionServer) Configure(arg0 context.Context, arg1 *apiextensions.CertConfig) (*apiextensions.HTTPAPIExtensionConfig, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Configure", arg0, arg1)
	ret0, _ := ret[0].(*apiextensions.HTTPAPIExtensionConfig)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Configure indicates an expected call of Configure.
func (mr *MockHTTPAPIExtensionServerMockRecorder) Configure(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Configure", reflect.TypeOf((*MockHTTPAPIExtensionServer)(nil).Configure), arg0, arg1)
}

// mustEmbedUnimplementedHTTPAPIExtensionServer mocks base method.
func (m *MockHTTPAPIExtensionServer) mustEmbedUnimplementedHTTPAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedHTTPAPIExtensionServer")
}

// mustEmbedUnimplementedHTTPAPIExtensionServer indicates an expected call of mustEmbedUnimplementedHTTPAPIExtensionServer.
func (mr *MockHTTPAPIExtensionServerMockRecorder) mustEmbedUnimplementedHTTPAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedHTTPAPIExtensionServer", reflect.TypeOf((*MockHTTPAPIExtensionServer)(nil).mustEmbedUnimplementedHTTPAPIExtensionServer))
}

// MockUnsafeHTTPAPIExtensionServer is a mock of UnsafeHTTPAPIExtensionServer interface.
type MockUnsafeHTTPAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockUnsafeHTTPAPIExtensionServerMockRecorder
}

// MockUnsafeHTTPAPIExtensionServerMockRecorder is the mock recorder for MockUnsafeHTTPAPIExtensionServer.
type MockUnsafeHTTPAPIExtensionServerMockRecorder struct {
	mock *MockUnsafeHTTPAPIExtensionServer
}

// NewMockUnsafeHTTPAPIExtensionServer creates a new mock instance.
func NewMockUnsafeHTTPAPIExtensionServer(ctrl *gomock.Controller) *MockUnsafeHTTPAPIExtensionServer {
	mock := &MockUnsafeHTTPAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockUnsafeHTTPAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnsafeHTTPAPIExtensionServer) EXPECT() *MockUnsafeHTTPAPIExtensionServerMockRecorder {
	return m.recorder
}

// mustEmbedUnimplementedHTTPAPIExtensionServer mocks base method.
func (m *MockUnsafeHTTPAPIExtensionServer) mustEmbedUnimplementedHTTPAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedHTTPAPIExtensionServer")
}

// mustEmbedUnimplementedHTTPAPIExtensionServer indicates an expected call of mustEmbedUnimplementedHTTPAPIExtensionServer.
func (mr *MockUnsafeHTTPAPIExtensionServerMockRecorder) mustEmbedUnimplementedHTTPAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedHTTPAPIExtensionServer", reflect.TypeOf((*MockUnsafeHTTPAPIExtensionServer)(nil).mustEmbedUnimplementedHTTPAPIExtensionServer))
}

// MockStreamAPIExtensionClient is a mock of StreamAPIExtensionClient interface.
type MockStreamAPIExtensionClient struct {
	ctrl     *gomock.Controller
	recorder *MockStreamAPIExtensionClientMockRecorder
}

// MockStreamAPIExtensionClientMockRecorder is the mock recorder for MockStreamAPIExtensionClient.
type MockStreamAPIExtensionClientMockRecorder struct {
	mock *MockStreamAPIExtensionClient
}

// NewMockStreamAPIExtensionClient creates a new mock instance.
func NewMockStreamAPIExtensionClient(ctrl *gomock.Controller) *MockStreamAPIExtensionClient {
	mock := &MockStreamAPIExtensionClient{ctrl: ctrl}
	mock.recorder = &MockStreamAPIExtensionClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamAPIExtensionClient) EXPECT() *MockStreamAPIExtensionClientMockRecorder {
	return m.recorder
}

// MockStreamAPIExtensionServer is a mock of StreamAPIExtensionServer interface.
type MockStreamAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockStreamAPIExtensionServerMockRecorder
}

// MockStreamAPIExtensionServerMockRecorder is the mock recorder for MockStreamAPIExtensionServer.
type MockStreamAPIExtensionServerMockRecorder struct {
	mock *MockStreamAPIExtensionServer
}

// NewMockStreamAPIExtensionServer creates a new mock instance.
func NewMockStreamAPIExtensionServer(ctrl *gomock.Controller) *MockStreamAPIExtensionServer {
	mock := &MockStreamAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockStreamAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStreamAPIExtensionServer) EXPECT() *MockStreamAPIExtensionServerMockRecorder {
	return m.recorder
}

// mustEmbedUnimplementedStreamAPIExtensionServer mocks base method.
func (m *MockStreamAPIExtensionServer) mustEmbedUnimplementedStreamAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedStreamAPIExtensionServer")
}

// mustEmbedUnimplementedStreamAPIExtensionServer indicates an expected call of mustEmbedUnimplementedStreamAPIExtensionServer.
func (mr *MockStreamAPIExtensionServerMockRecorder) mustEmbedUnimplementedStreamAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedStreamAPIExtensionServer", reflect.TypeOf((*MockStreamAPIExtensionServer)(nil).mustEmbedUnimplementedStreamAPIExtensionServer))
}

// MockUnsafeStreamAPIExtensionServer is a mock of UnsafeStreamAPIExtensionServer interface.
type MockUnsafeStreamAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockUnsafeStreamAPIExtensionServerMockRecorder
}

// MockUnsafeStreamAPIExtensionServerMockRecorder is the mock recorder for MockUnsafeStreamAPIExtensionServer.
type MockUnsafeStreamAPIExtensionServerMockRecorder struct {
	mock *MockUnsafeStreamAPIExtensionServer
}

// NewMockUnsafeStreamAPIExtensionServer creates a new mock instance.
func NewMockUnsafeStreamAPIExtensionServer(ctrl *gomock.Controller) *MockUnsafeStreamAPIExtensionServer {
	mock := &MockUnsafeStreamAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockUnsafeStreamAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnsafeStreamAPIExtensionServer) EXPECT() *MockUnsafeStreamAPIExtensionServerMockRecorder {
	return m.recorder
}

// mustEmbedUnimplementedStreamAPIExtensionServer mocks base method.
func (m *MockUnsafeStreamAPIExtensionServer) mustEmbedUnimplementedStreamAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedStreamAPIExtensionServer")
}

// mustEmbedUnimplementedStreamAPIExtensionServer indicates an expected call of mustEmbedUnimplementedStreamAPIExtensionServer.
func (mr *MockUnsafeStreamAPIExtensionServerMockRecorder) mustEmbedUnimplementedStreamAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedStreamAPIExtensionServer", reflect.TypeOf((*MockUnsafeStreamAPIExtensionServer)(nil).mustEmbedUnimplementedStreamAPIExtensionServer))
}

// MockUnaryAPIExtensionClient is a mock of UnaryAPIExtensionClient interface.
type MockUnaryAPIExtensionClient struct {
	ctrl     *gomock.Controller
	recorder *MockUnaryAPIExtensionClientMockRecorder
}

// MockUnaryAPIExtensionClientMockRecorder is the mock recorder for MockUnaryAPIExtensionClient.
type MockUnaryAPIExtensionClientMockRecorder struct {
	mock *MockUnaryAPIExtensionClient
}

// NewMockUnaryAPIExtensionClient creates a new mock instance.
func NewMockUnaryAPIExtensionClient(ctrl *gomock.Controller) *MockUnaryAPIExtensionClient {
	mock := &MockUnaryAPIExtensionClient{ctrl: ctrl}
	mock.recorder = &MockUnaryAPIExtensionClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnaryAPIExtensionClient) EXPECT() *MockUnaryAPIExtensionClientMockRecorder {
	return m.recorder
}

// UnaryDescriptor mocks base method.
func (m *MockUnaryAPIExtensionClient) UnaryDescriptor(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*descriptorpb.ServiceDescriptorProto, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, in}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "UnaryDescriptor", varargs...)
	ret0, _ := ret[0].(*descriptorpb.ServiceDescriptorProto)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UnaryDescriptor indicates an expected call of UnaryDescriptor.
func (mr *MockUnaryAPIExtensionClientMockRecorder) UnaryDescriptor(ctx, in interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, in}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UnaryDescriptor", reflect.TypeOf((*MockUnaryAPIExtensionClient)(nil).UnaryDescriptor), varargs...)
}

// MockUnaryAPIExtensionServer is a mock of UnaryAPIExtensionServer interface.
type MockUnaryAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockUnaryAPIExtensionServerMockRecorder
}

// MockUnaryAPIExtensionServerMockRecorder is the mock recorder for MockUnaryAPIExtensionServer.
type MockUnaryAPIExtensionServerMockRecorder struct {
	mock *MockUnaryAPIExtensionServer
}

// NewMockUnaryAPIExtensionServer creates a new mock instance.
func NewMockUnaryAPIExtensionServer(ctrl *gomock.Controller) *MockUnaryAPIExtensionServer {
	mock := &MockUnaryAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockUnaryAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnaryAPIExtensionServer) EXPECT() *MockUnaryAPIExtensionServerMockRecorder {
	return m.recorder
}

// UnaryDescriptor mocks base method.
func (m *MockUnaryAPIExtensionServer) UnaryDescriptor(arg0 context.Context, arg1 *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnaryDescriptor", arg0, arg1)
	ret0, _ := ret[0].(*descriptorpb.ServiceDescriptorProto)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UnaryDescriptor indicates an expected call of UnaryDescriptor.
func (mr *MockUnaryAPIExtensionServerMockRecorder) UnaryDescriptor(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UnaryDescriptor", reflect.TypeOf((*MockUnaryAPIExtensionServer)(nil).UnaryDescriptor), arg0, arg1)
}

// mustEmbedUnimplementedUnaryAPIExtensionServer mocks base method.
func (m *MockUnaryAPIExtensionServer) mustEmbedUnimplementedUnaryAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedUnaryAPIExtensionServer")
}

// mustEmbedUnimplementedUnaryAPIExtensionServer indicates an expected call of mustEmbedUnimplementedUnaryAPIExtensionServer.
func (mr *MockUnaryAPIExtensionServerMockRecorder) mustEmbedUnimplementedUnaryAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedUnaryAPIExtensionServer", reflect.TypeOf((*MockUnaryAPIExtensionServer)(nil).mustEmbedUnimplementedUnaryAPIExtensionServer))
}

// MockUnsafeUnaryAPIExtensionServer is a mock of UnsafeUnaryAPIExtensionServer interface.
type MockUnsafeUnaryAPIExtensionServer struct {
	ctrl     *gomock.Controller
	recorder *MockUnsafeUnaryAPIExtensionServerMockRecorder
}

// MockUnsafeUnaryAPIExtensionServerMockRecorder is the mock recorder for MockUnsafeUnaryAPIExtensionServer.
type MockUnsafeUnaryAPIExtensionServerMockRecorder struct {
	mock *MockUnsafeUnaryAPIExtensionServer
}

// NewMockUnsafeUnaryAPIExtensionServer creates a new mock instance.
func NewMockUnsafeUnaryAPIExtensionServer(ctrl *gomock.Controller) *MockUnsafeUnaryAPIExtensionServer {
	mock := &MockUnsafeUnaryAPIExtensionServer{ctrl: ctrl}
	mock.recorder = &MockUnsafeUnaryAPIExtensionServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockUnsafeUnaryAPIExtensionServer) EXPECT() *MockUnsafeUnaryAPIExtensionServerMockRecorder {
	return m.recorder
}

// mustEmbedUnimplementedUnaryAPIExtensionServer mocks base method.
func (m *MockUnsafeUnaryAPIExtensionServer) mustEmbedUnimplementedUnaryAPIExtensionServer() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "mustEmbedUnimplementedUnaryAPIExtensionServer")
}

// mustEmbedUnimplementedUnaryAPIExtensionServer indicates an expected call of mustEmbedUnimplementedUnaryAPIExtensionServer.
func (mr *MockUnsafeUnaryAPIExtensionServerMockRecorder) mustEmbedUnimplementedUnaryAPIExtensionServer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "mustEmbedUnimplementedUnaryAPIExtensionServer", reflect.TypeOf((*MockUnsafeUnaryAPIExtensionServer)(nil).mustEmbedUnimplementedUnaryAPIExtensionServer))
}

package gateway

import (
	"context"
	"math"
	"sync"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/kralicky/totem"
	agentv1 "github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type DelegateServer struct {
	DelegateServerOptions
	uuid             string
	config           v1beta1.GatewayConfigSpec
	mu               *sync.RWMutex
	rcond            *sync.Cond
	localAgents      map[string]grpc.ClientConnInterface
	logger           *slog.Logger
	clusterStore     storage.ClusterStore
	relayDialer      *instanceDialer
	managementDialer *instanceDialer
}

type DelegateServerOptions struct {
	ct *ConnectionTracker
}

type DelegateServerOption func(*DelegateServerOptions)

func (o *DelegateServerOptions) apply(opts ...DelegateServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithConnectionTracker(connectionTracker *ConnectionTracker) DelegateServerOption {
	return func(o *DelegateServerOptions) {
		o.ct = connectionTracker
	}
}

func NewDelegateServer(config v1beta1.GatewayConfigSpec, clusterStore storage.ClusterStore, lg *slog.Logger, opts ...DelegateServerOption) *DelegateServer {
	options := DelegateServerOptions{}
	options.apply(opts...)
	mu := &sync.RWMutex{}
	return &DelegateServer{
		DelegateServerOptions: options,
		uuid:                  uuid.NewString(),
		config:                config,
		mu:                    mu,
		rcond:                 sync.NewCond(mu.RLocker()),
		localAgents:           make(map[string]grpc.ClientConnInterface),
		clusterStore:          clusterStore,
		logger:                lg.WithGroup("delegate"),
		relayDialer:           newDialer(serverRelay),
		managementDialer:      newDialer(serverManagement),
	}
}

func (d *DelegateServer) HandleAgentConnection(ctx context.Context, clientSet agentv1.ClientSet) {
	d.mu.Lock()
	id := cluster.StreamAuthorizedID(ctx)

	d.localAgents[id] = clientSet.ClientConn()
	d.logger.With("id", id).Debug("agent connected")
	d.mu.Unlock()
	d.rcond.Broadcast()

	<-ctx.Done()

	d.mu.Lock()
	delete(d.localAgents, id)
	d.logger.With("id", id).Debug("agent disconnected")
	d.mu.Unlock()
	d.rcond.Broadcast()
}

func (d *DelegateServer) Request(ctx context.Context, req *streamv1.DelegatedMessage) (*totem.Response, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	targetId := req.GetTarget().GetId()
	lg := d.logger.With(
		"target", targetId,
		"request", req.GetRequest().QualifiedMethodName(),
	)
	lg.Debug("delegating rpc request")
ATTEMPT:
	for {
		target, ok := d.localAgents[targetId]
		if ok {
			fwdResp := &totem.RPC{}
			err := target.Invoke(ctx, totem.Forward, req.GetRequest(), fwdResp)
			if err != nil {
				// the actual request failed (network error, etc.)
				d.logger.With(
					logger.Err(err),
				).Error("delegating rpc request failed (local)")
				return nil, err
			}
			return fwdResp.GetResponse(), nil
			// if fwdResp.GetResponse().GetStatus().Err() != nil {
			// 	d.logger.With(
			// 		logger.Err(err),
			// 	).Error("delegating rpc request failed (remote)")
			// 	// the request failed, but somewhere on the remote side
			// 	return nil, fwdResp.GetResponse().GetStatus().Err()
			// }

			// // unwrap the inner response
			// resp := &totem.RPC{}
			// err = proto.Unmarshal(fwdResp.GetResponse().GetResponse(), resp)
			// if err != nil {
			// 	d.logger.With(
			// 		logger.Err(err),
			// 	).Error("malformed rpc response from agent")
			// 	return nil, err
			// }

			// return fwdResp.GetResponse(), nil
		}
		// check the connection tracker

		if info := d.ct.Lookup(targetId); info != nil {
			if d.ct.IsLocalInstance(info) {
				// if the target is local, we might be waiting on a write lock for d.mu
				// to be acquired, which would add the target we're looking for to
				// d.activeAgents.
				if !d.mu.TryRLock() {
					// unlock the read lock acquired at the beginning of this function,
					// and wait to be signalled by HandleAgentConnection
					d.rcond.Wait()
					if _, ok := d.localAgents[targetId]; ok {
						continue ATTEMPT
					}
				} else {
					// we're out of sync, tell the client to retry
					d.mu.RUnlock() // d.mu is read-locked twice, unlock one (other unlock is deferred)
					return nil, status.Error(codes.Unavailable, "agent connection state is not consistent (try again later)")
				}
			} else {
				lg.Debug("relaying request to controlling instance")
				cc, err := d.relayDialer.Dial(info)
				if err != nil {
					return nil, status.Errorf(codes.Unavailable, "failed to dial controlling instance: %s", err.Error())
				}
				relayClient := streamv1.NewRelayClient(cc)
				resp, err := relayClient.RelayDelegateRequest(ctx, &streamv1.RelayedDelegatedMessage{
					Message:   req,
					Principal: d.ct.LocalInstanceInfo(),
				})
				if err != nil {
					return nil, err
				}
				return resp, nil
			}
			break
		}
	}

	err := status.Error(codes.NotFound, "target not found")
	lg.With(
		logger.Err(err),
	).Warn("delegating rpc request failed")
	return nil, err
}

func (d *DelegateServer) Broadcast(ctx context.Context, req *streamv1.BroadcastMessage) (*streamv1.BroadcastReplyList, error) {
	targets := d.ct.ListActiveConnections()
	if len(targets) == 0 {
		return &streamv1.BroadcastReplyList{}, nil
	}
	if len(req.GetTargetSelector().GetTargets()) > 0 {
		selected := map[string]struct{}{}
		for _, s := range req.GetTargetSelector().GetTargets() {
			selected[s.GetId()] = struct{}{}
		}
		filtered := make([]string, 0, len(targets))
		for _, target := range targets {
			if _, ok := selected[target]; ok {
				filtered = append(filtered, target)
			}
		}
		targets = filtered
	}

	reply := &streamv1.BroadcastReplyList{
		Items: make([]*streamv1.BroadcastReply, 0, len(targets)),
	}
	for _, target := range targets {
		entry := &streamv1.BroadcastReply{
			Target: &corev1.Reference{
				Id: target,
			},
		}
		var err error
		entry.Response, err = d.Request(ctx, &streamv1.DelegatedMessage{
			Request: req.GetRequest(),
			Target:  &corev1.Reference{Id: target},
		})
		if err != nil {
			// this will be logged in the Request call
			continue
		}
		reply.Items = append(reply.Items, entry)
	}

	d.logger.With(
		"targets", len(targets),
	).Debug("broadcasted rpc request")

	return reply, nil
}

func (d *DelegateServer) NewRelayServer() *RelayServer {
	if d.ct == nil {
		panic("relay server requires a connection tracker")
	}
	return &RelayServer{
		delegateSrv:       d,
		listenAddress:     d.config.Management.RelayListenAddress,
		connectionTracker: d.ct,
		logger:            d.logger.WithGroup("relay"),
	}
}

type RelayServer struct {
	connectionTracker *ConnectionTracker
	listenAddress     string
	delegateSrv       *DelegateServer
	logger            *slog.Logger
}

func (rs *RelayServer) ListenAndServe(ctx context.Context) error {
	listener, err := util.NewProtocolListener(rs.listenAddress)
	if err != nil {
		return err
	}

	creds := insecure.NewCredentials() // todo?

	server := grpc.NewServer(
		grpc.Creds(creds),
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.MaxSendMsgSize(math.MaxInt32),
		// note: no idle timeout config here, it's handled client-side.
		// also no pooled stream workers, the delegate server should use few
		// resources when idle, and can burst when needed
	)
	streamv1.RegisterRelayServer(server, rs)

	errC := lo.Async(func() error {
		return server.Serve(listener)
	})

	select {
	case <-ctx.Done():
		server.Stop()
		return ctx.Err()
	case err := <-errC:
		return err
	}
}

var ErrLocalTarget = status.Error(codes.FailedPrecondition, "requested target is controlled by the local instance")

func (d *DelegateServer) NewTargetedManagementClient(target *corev1.Reference) (managementv1.ManagementClient, error) {
	info := d.ct.Lookup(target.GetId())
	if info == nil {
		return nil, status.Errorf(codes.NotFound, "target %q not found", target.GetId())
	}
	d.logger.With("target", target.GetId(), "info", info.String()).Debug("dialing management client")
	if d.ct.IsLocalInstance(info) {
		d.logger.With("target", target.GetId()).Debug("target is local")
		return nil, ErrLocalTarget
	}
	d.logger.With("target", target.GetId()).Debug("target is remote")
	cc, err := d.managementDialer.Dial(info)
	if err != nil {
		return nil, err
	}
	return managementv1.NewManagementClient(cc), nil
}

// HandleDelegateRequest implements v1.RelayServer.
func (rs *RelayServer) RelayDelegateRequest(ctx context.Context, msg *streamv1.RelayedDelegatedMessage) (*totem.Response, error) {
	principal := msg.GetPrincipal()
	target := msg.GetMessage().GetTarget().GetId()
	if rs.connectionTracker.IsLocalInstance(principal) {
		panic("bug: attempted to relay a delegated request to self")
	}
	rs.logger.Debug("delegating request on behalf of " + principal.GetRelayAddress())
	if !rs.connectionTracker.IsTrackedLocal(target) {
		return nil, status.Error(codes.FailedPrecondition, "requested target is not controlled by this instance")
	}
	resp, err := rs.delegateSrv.Request(ctx, msg.GetMessage())
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type serverKind int

const (
	serverRelay serverKind = iota
	serverManagement
)

// Manages connections to multiple relay servers efficiently.
type instanceDialer struct {
	mu      sync.RWMutex
	clients map[string]*grpc.ClientConn
	dialSf  singleflight.Group
	server  serverKind
}

func newDialer(server serverKind) *instanceDialer {
	return &instanceDialer{
		clients: make(map[string]*grpc.ClientConn),
		server:  server,
	}
}

func (d *instanceDialer) Dial(info *corev1.InstanceInfo) (grpc.ClientConnInterface, error) {
	d.mu.RLock()
	var addr string
	switch d.server {
	case serverRelay:
		addr = info.GetRelayAddress()
	case serverManagement:
		addr = info.GetManagementAddress()
	}
	if cc, ok := d.clients[addr]; ok {
		state := cc.GetState()
		if state != connectivity.Shutdown {
			switch state {
			case connectivity.Idle:
				cc.Connect()
			case connectivity.TransientFailure:
				cc.ResetConnectBackoff()
			}
			return cc, nil
		}
	}
	d.mu.RUnlock()

	res, err, _ := d.dialSf.Do(addr, func() (any, error) {
		cc, err := grpc.Dial(addr,
			// A short client-side idle timeout will aggressively force the connection
			// to go idle when it is not actively being used, freeing up resources on
			// the other end. Because any given instance can communicate with every
			// other instance, this should significantly limit the number of actual
			// open connections. The servers do not enforce idle timeouts themselves.
			grpc.WithIdleTimeout(5*time.Second),
			grpc.WithContextDialer(util.DialProtocol),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(math.MaxInt32),
				grpc.MaxCallSendMsgSize(math.MaxInt32),
				grpc.WaitForReady(true),
			),
		)
		if err != nil {
			return nil, err
		}
		// still need to acquire the write lock here, note the read lock has been
		// released earlier
		d.mu.Lock()
		d.clients[addr] = cc
		d.mu.Unlock()
		return cc, nil
	})
	return res.(*grpc.ClientConn), err
}

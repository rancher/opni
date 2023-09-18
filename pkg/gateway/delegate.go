package gateway

import (
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/kralicky/totem"
	agentv1 "github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

type agentInfo struct {
	grpc.ClientConnInterface

	labels map[string]string
	id     string
}

func (a agentInfo) GetLabels() map[string]string {
	return a.labels
}

func (a agentInfo) GetId() string {
	return a.id
}

type DelegateServer struct {
	DelegateServerOptions
	streamv1.UnsafeDelegateServer
	streamv1.UnsafeRelayServer
	config       v1beta1.GatewayConfigSpec
	mu           *sync.RWMutex
	rcond        *sync.Cond
	activeAgents map[string]agentInfo
	logger       *zap.SugaredLogger
	clusterStore storage.ClusterStore
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

func NewDelegateServer(config v1beta1.GatewayConfigSpec, clusterStore storage.ClusterStore, lg *zap.SugaredLogger, opts ...DelegateServerOption) *DelegateServer {
	options := DelegateServerOptions{}
	options.apply(opts...)
	mu := &sync.RWMutex{}
	return &DelegateServer{
		DelegateServerOptions: options,
		config:                config,
		mu:                    mu,
		rcond:                 sync.NewCond(mu.RLocker()),
		activeAgents:          make(map[string]agentInfo),
		clusterStore:          clusterStore,
		logger:                lg.Named("delegate"),
	}
}

func (d *DelegateServer) HandleAgentConnection(ctx context.Context, clientSet agentv1.ClientSet) {
	d.mu.Lock()
	id := cluster.StreamAuthorizedID(ctx)

	cluster, err := d.clusterStore.GetCluster(ctx, &corev1.Reference{Id: id})
	if err != nil {
		d.logger.With(
			"id", id,
			zap.Error(err),
		).Error("internal error: failed to look up connecting agent")
		d.mu.Unlock()
		d.rcond.Broadcast()
		return
	}

	d.activeAgents[id] = agentInfo{
		ClientConnInterface: clientSet.ClientConn(),
		labels:              cluster.GetLabels(),
		id:                  id,
	}
	d.logger.With("id", id).Debug("agent connected")
	d.mu.Unlock()
	d.rcond.Broadcast()

	<-ctx.Done()

	d.mu.Lock()
	delete(d.activeAgents, id)
	d.logger.With("id", id).Debug("agent disconnected")
	d.mu.Unlock()
	d.rcond.Broadcast()
}

func (d *DelegateServer) Request(ctx context.Context, req *streamv1.DelegatedMessage) (*totem.RPC, error) {
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
		target, ok := d.activeAgents[targetId]
		if ok {
			fwdResp := &totem.RPC{}
			err := target.Invoke(ctx, totem.Forward, req.GetRequest(), fwdResp)
			if err != nil {
				d.logger.With(
					zap.Error(err),
				).Error("delegating rpc request failed")
				return nil, err
			}

			resp := &totem.RPC{}
			err = proto.Unmarshal(fwdResp.GetResponse().GetResponse(), resp)
			if err != nil {
				d.logger.With(
					zap.Error(err),
				).Error("delegating rpc request failed")
				return nil, err
			}

			return resp, nil
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
					if _, ok := d.activeAgents[targetId]; ok {
						continue ATTEMPT
					}
				} else {
					// we're out of sync, tell the client to retry
					d.mu.RUnlock() // d.mu is read-locked twice, unlock one (other unlock is deferred)
					return nil, status.Error(codes.Unavailable, "agent connection state is not consistent (try again later)")
				}
			} else {
				lg.Debug("relaying request to controlling instance")
				addr := info.GetRelayAddress()
				cc, err := grpc.DialContext(ctx, addr,
					grpc.WithContextDialer(util.DialProtocol),
					grpc.WithBlock(),
					grpc.FailOnNonTempDialError(true),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return nil, status.Errorf(codes.Unavailable, "failed to dial controlling instance: %s", err.Error())
				}
				defer cc.Close() // TODO: try keeping this open for a short time
				relayClient := streamv1.NewRelayClient(cc)
				resp, err := relayClient.RelayDelegateRequest(ctx, &streamv1.RelayedDelegatedMessage{
					Message:   req,
					Principal: d.ct.LocalInstanceInfo(),
				}, grpc.WaitForReady(true))
				if err != nil {
					return nil, err
				}
				return resp.GetReply(), status.ErrorProto(resp.GetStatus())
			}
			break
		}
	}

	err := status.Error(codes.NotFound, "target not found")
	lg.With(
		zap.Error(err),
	).Warn("delegating rpc request failed")
	return nil, err
}

func (d *DelegateServer) Broadcast(ctx context.Context, req *streamv1.BroadcastMessage) (*streamv1.BroadcastReplyList, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sp := storage.NewSelectorPredicate[agentInfo](req.GetTargetSelector())

	var targets []agentInfo
	for _, aa := range d.activeAgents {
		if sp(aa) {
			targets = append(targets, aa)
		}
	}

	if len(targets) == 0 {
		return nil, status.Error(codes.NotFound, "no targets found")
	}

	eg, ctx := errgroup.WithContext(ctx)
	reply := &streamv1.BroadcastReplyList{
		Responses: make([]*streamv1.BroadcastReply, len(targets)),
	}

	for i, target := range targets {
		i, target := i, target
		eg.Go(func() error {
			item := &totem.RPC{}
			if err := target.Invoke(ctx, totem.Forward, req.GetRequest(), item); err != nil {
				return err
			}

			reply.Responses[i] = &streamv1.BroadcastReply{
				Ref: &corev1.Reference{
					Id: target.id,
				},
				Reply: item,
			}

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return nil, err
	}

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
		logger:            d.logger.Named("relay"),
	}
}

type RelayServer struct {
	streamv1.UnsafeRelayServer
	connectionTracker *ConnectionTracker
	listenAddress     string
	delegateSrv       *DelegateServer
	logger            *zap.SugaredLogger
}

func (rs *RelayServer) ListenAndServe(ctx context.Context) error {
	listener, err := util.NewProtocolListener(rs.listenAddress)
	if err != nil {
		return err
	}

	creds := insecure.NewCredentials() // todo

	server := grpc.NewServer(
		grpc.Creds(creds),
		grpc.NumStreamWorkers(uint32(runtime.NumCPU())),
		grpc.MaxRecvMsgSize(math.MaxInt32),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 10 * time.Second,
		}),
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

// HandleDelegateRequest implements v1.RelayServer.
func (rs *RelayServer) RelayDelegateRequest(ctx context.Context, msg *streamv1.RelayedDelegatedMessage) (*streamv1.DelegatedMessageReply, error) {
	principal := msg.GetPrincipal()
	target := msg.GetMessage().GetTarget().GetId()
	if rs.connectionTracker.IsLocalInstance(principal) {
		panic("bug: attempted to relay a delegated request to self")
	}
	rs.logger.Debugf("delegating request on behalf of %s", principal)
	if !rs.connectionTracker.IsTrackedLocal(target) {
		return nil, status.Error(codes.FailedPrecondition, "requested target is not controlled by this instance")
	}
	rpc, err := rs.delegateSrv.Request(ctx, msg.GetMessage())
	return &streamv1.DelegatedMessageReply{
		Reply:  rpc,
		Status: status.Convert(err).Proto(),
	}, nil
}

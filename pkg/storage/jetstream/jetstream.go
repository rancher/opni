package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"github.com/iancoleman/strcase"
	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/runtime/protoiface"
)

const (
	tokensBucket       = "tokens"
	clustersBucket     = "clusters"
	keyringsBucket     = "keyrings"
	rolesBucket        = "roles"
	roleBindingsBucket = "rolebindings"
	dynamicBucket      = "dynamic"
)

type JetStreamStore struct {
	JetStreamStoreOptions

	// controls the lifetime of the store connection; cancel to disconnect
	ctx context.Context

	nc *nats.Conn
	js nats.JetStreamContext

	kv struct {
		Tokens       nats.KeyValue
		Clusters     nats.KeyValue
		Keyrings     nats.KeyValue
		Roles        nats.KeyValue
		RoleBindings nats.KeyValue
	}
	logger *slog.Logger
}

var _ storage.Backend = (*JetStreamStore)(nil)

type JetStreamStoreOptions struct {
	BucketPrefix string
}

type JetStreamStoreOption func(*JetStreamStoreOptions)

func (o *JetStreamStoreOptions) apply(opts ...JetStreamStoreOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBucketPrefix(prefix string) JetStreamStoreOption {
	return func(o *JetStreamStoreOptions) {
		o.BucketPrefix = prefix
	}
}

func NewJetStreamStore(ctx context.Context, conf *v1beta1.JetStreamStorageSpec, opts ...JetStreamStoreOption) (*JetStreamStore, error) {
	options := JetStreamStoreOptions{
		BucketPrefix: "gateway",
	}
	options.apply(opts...)

	lg := logger.New(logger.WithLogLevel(slog.LevelWarn)).WithGroup("jetstream")

	nkeyOpt, err := nats.NkeyOptionFromSeed(conf.NkeySeedPath)
	if err != nil {
		return nil, err
	}
	nc, err := nats.Connect(conf.Endpoint,
		nkeyOpt,
		nats.MaxReconnects(-1),
		nats.RetryOnFailedConnect(true),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			if err == nil {
				lg.Debug("jetstream client closed")
				return
			}
			lg.With(
				zap.Error(err),
			).Warn("disconnected from jetstream")
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			lg.With(
				"server", c.ConnectedAddr(),
				"id", c.ConnectedServerId(),
				"name", c.ConnectedServerName(),
				"version", c.ConnectedServerVersion(),
			).Info("reconnected to jetstream")
		}),
	)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		nc.Close()
	}()

	ctrl := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(10*time.Millisecond),
		backoff.WithMaxInterval(10*time.Millisecond<<9),
		backoff.WithMultiplier(2.0),
	).Start(ctx)
	for {
		if rtt, err := nc.RTT(); err == nil {
			lg.With("rtt", rtt).Info("nats server connection is healthy")
			break
		}
		select {
		case <-ctrl.Done():
			return nil, ctx.Err()
		case <-ctrl.Next():
		}
	}

	js, err := nc.JetStream(nats.Context(ctx))
	if err != nil {
		return nil, err
	}

	store := &JetStreamStore{
		JetStreamStoreOptions: options,
		ctx:                   ctx,
		nc:                    nc,
		js:                    js,
		logger:                lg,
	}

	store.kv.Tokens = store.upsertBucket(tokensBucket)
	store.kv.Clusters = store.upsertBucket(clustersBucket)
	store.kv.Keyrings = store.upsertBucket(keyringsBucket)
	store.kv.Roles = store.upsertBucket(rolesBucket)
	store.kv.RoleBindings = store.upsertBucket(roleBindingsBucket)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return store, nil
}

func (s *JetStreamStore) upsertBucket(name string) nats.KeyValue {
	bucketName := fmt.Sprintf("%s-%s", s.BucketPrefix, name)
	for s.ctx.Err() == nil {
		kv, err := s.js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket: bucketName,
			Description: fmt.Sprintf("Opni %s %s Store",
				strcase.ToCamel(s.BucketPrefix),
				strcase.ToCamel(name)),
			Storage:  nats.FileStorage,
			History:  64,
			Replicas: 1,
		})
		if err != nil {
			s.logger.With(
				"bucket", bucketName,
				zap.Error(err),
			).Warn("failed to create bucket, retrying")
			continue
		}
		return kv
	}
	s.logger.With(
		"bucket", bucketName,
		zap.Error(s.ctx.Err()),
	).Error("failed to create bucket")
	return nil
}

func (s *JetStreamStore) KeyringStore(prefix string, ref *corev1.Reference) storage.KeyringStore {
	return &jetstreamKeyringStore{
		kv:     s.kv.Keyrings,
		ref:    ref,
		prefix: prefix,
	}
}

func (s *JetStreamStore) KeyValueStore(prefix string) storage.KeyValueStore {
	// sanitize bucket name
	prefix = strings.ReplaceAll(strings.ReplaceAll(prefix, "/", "-"), ".", "_")
	bucket := s.upsertBucket(fmt.Sprintf("%s-%s", dynamicBucket, prefix))
	return &jetstreamKeyValueStore{
		kv: bucket,
	}
}

// here be dragons
func jetstreamGrpcError(err error) error {
	if err == nil {
		return nil
	}
	code := codes.Unknown
	details := []protoiface.MessageV1{}
	switch err {
	case nats.ErrKeyValueConfigRequired, nats.ErrInvalidBucketName, nats.ErrInvalidKey, nats.ErrBadBucket, nats.ErrHistoryToLarge:
		code = codes.InvalidArgument
	case nats.ErrBucketNotFound, nats.ErrKeyNotFound, nats.ErrKeyDeleted, nats.ErrNoKeysFound:
		code = codes.NotFound
	default:
		var jserr nats.JetStreamError
		if errors.As(err, &jserr) {
			apierror := jserr.APIError()
			switch apierror.ErrorCode {
			case nats.JSErrCodeJetStreamNotEnabledForAccount, nats.JSErrCodeJetStreamNotEnabled:
				code = codes.PermissionDenied
			case nats.JSErrCodeInsufficientResourcesErr:
				code = codes.ResourceExhausted
			case nats.JSErrCodeStreamNotFound, nats.JSErrCodeConsumerNotFound, nats.JSErrCodeMessageNotFound:
				code = codes.NotFound
			case nats.JSErrCodeStreamNameInUse, nats.JSErrCodeConsumerNameExists, nats.JSErrCodeConsumerAlreadyExists:
				code = codes.AlreadyExists
			case nats.JSErrCodeBadRequest:
				code = codes.InvalidArgument
			case nats.JSErrCodeStreamWrongLastSequence:
				if err.Error() == nats.ErrKeyExists.Error() {
					// this error code is overloaded, only way to differentiate
					// is by comparing the error message on the original wrapped error
					code = codes.AlreadyExists
				} else {
					code = codes.Aborted
					details = append(details, storage.ErrDetailsConflict)
				}
			}
		}
	}
	se := status.New(code, err.Error())
	if len(details) > 0 {
		if errWithDetails, err := se.WithDetails(details...); err == nil {
			se = errWithDetails
		}
	}
	return se.Err()
}

func init() {
	storage.RegisterStoreBuilder(v1beta1.StorageTypeJetStream, func(args ...any) (any, error) {
		ctx := args[0].(context.Context)
		conf := args[1].(*v1beta1.JetStreamStorageSpec)

		var opts []JetStreamStoreOption
		for _, arg := range args[2:] {
			switch v := arg.(type) {
			case string:
				opts = append(opts, WithBucketPrefix(v))
			default:
				return nil, fmt.Errorf("unexpected argument: %v", arg)
			}
		}

		return NewJetStreamStore(ctx, conf, opts...)
	})
}

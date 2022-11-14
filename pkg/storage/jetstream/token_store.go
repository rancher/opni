package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *JetStreamStore) CreateToken(ctx context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*corev1.BootstrapToken, error) {
	options := storage.NewTokenCreateOptions()
	options.Apply(opts...)

	token := tokens.NewToken().ToBootstrapToken()
	token.Metadata = &corev1.BootstrapTokenMetadata{
		LeaseID:      -1,
		Ttl:          int64(ttl.Seconds()),
		UsageCount:   0,
		Labels:       options.Labels,
		Capabilities: options.Capabilities,
	}
	data, err := protojson.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token: %w", err)
	}

	rev, err := s.kv.Tokens.Create(token.TokenID, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}
	token.SetResourceVersion(fmt.Sprint(rev))
	return token, nil
}

func (s *JetStreamStore) DeleteToken(ctx context.Context, ref *corev1.Reference) error {
	_, err := s.GetToken(ctx, ref)
	if err != nil {
		return err
	}
	return s.kv.Tokens.Delete(ref.Id)
}

func (s *JetStreamStore) GetToken(ctx context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, error) {
	token := &corev1.BootstrapToken{}
	entry, err := s.kv.Tokens.Get(ref.Id)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	if err := protojson.Unmarshal(entry.Value(), token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}
	patchTTL(token, entry)
	if token.Metadata.Ttl <= 0 {
		go s.garbageCollectToken(token)
		return nil, storage.ErrNotFound
	}
	token.SetResourceVersion(fmt.Sprint(entry.Revision()))
	return token, nil
}

func (s *JetStreamStore) UpdateToken(ctx context.Context, ref *corev1.Reference, mutator storage.TokenMutator) (*corev1.BootstrapToken, error) {
	p := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(1*time.Millisecond),
		backoff.WithMaxInterval(128*time.Millisecond),
		backoff.WithMultiplier(2),
	)
	b := p.Start(ctx)
	var updateErr error
	for backoff.Continue(b) {
		token, err := s.GetToken(ctx, ref)
		if err != nil {
			return nil, err
		}
		mutator(token)
		data, err := protojson.Marshal(token)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal token: %w", err)
		}
		versionStr := token.GetResourceVersion()
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse resource version: %w", err)
		}
		rev, err := s.kv.Tokens.Update(ref.Id, data, version)
		if err != nil {
			updateErr = err
			continue
		}
		token.SetResourceVersion(fmt.Sprint(rev))
		return token, nil
	}
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update token: %w", updateErr)
	}
	return nil, fmt.Errorf("failed to update token: (unknown error)")
}

func (s *JetStreamStore) ListTokens(ctx context.Context) ([]*corev1.BootstrapToken, error) {
	watcher, err := s.kv.Tokens.WatchAll(nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	var tokens []*corev1.BootstrapToken
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}

		token := &corev1.BootstrapToken{}
		if err := protojson.Unmarshal(entry.Value(), token); err != nil {
			return nil, fmt.Errorf("failed to unmarshal token: %w", err)
		}
		patchTTL(token, entry)
		if token.Metadata.Ttl <= 0 {
			go s.garbageCollectToken(token)
			continue
		}
		token.SetResourceVersion(fmt.Sprint(entry.Revision()))
		tokens = append(tokens, token)
	}

	return tokens, nil
}

func patchTTL(token *corev1.BootstrapToken, entry nats.KeyValueEntry) {
	created := entry.Created()
	ttl := token.Metadata.Ttl
	// edit the ttl to reflect the current ttl of the token
	newTtl := ttl - (time.Now().Unix() - created.Unix())
	if newTtl < 0 {
		newTtl = 0
	}
	token.Metadata.Ttl = newTtl
}

// garbageCollectToken performs a best-effort deletion of an expired token.
func (s *JetStreamStore) garbageCollectToken(token *corev1.BootstrapToken) {
	s.logger.With(
		"token", token.TokenID,
	).Debug("garbage-collecting expired token")
	if err := s.kv.Tokens.Delete(token.TokenID); err != nil {
		if !errors.Is(err, nats.ErrKeyNotFound) {
			s.logger.With(
				"token", token.TokenID,
				"error", err,
			).Warn("failed to garbage-collect expired token")
		}
	}
}

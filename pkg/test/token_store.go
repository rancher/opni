package test

import (
	"context"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

func NewTestTokenStore(ctx context.Context, ctrl *gomock.Controller) storage.TokenStore {
	mockTokenStore := mock_storage.NewMockTokenStore(ctrl)

	leaseStore := NewLeaseStore(ctx)
	tks := map[string]*core.BootstrapToken{}
	mu := sync.Mutex{}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tokenID := <-leaseStore.LeaseExpired():
				mockTokenStore.DeleteToken(ctx, &core.Reference{
					Id: tokenID,
				})
			}
		}
	}()

	mockTokenStore.EXPECT().
		CreateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*core.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			options := storage.NewTokenCreateOptions()
			options.Apply(opts...)
			t := tokens.NewToken().ToBootstrapToken()
			lease := leaseStore.New(t.TokenID, ttl)
			t.Metadata = &core.BootstrapTokenMetadata{
				LeaseID:      int64(lease.ID),
				Ttl:          int64(ttl),
				UsageCount:   0,
				Labels:       options.Labels,
				Capabilities: options.Capabilities,
			}
			tks[t.TokenID] = t
			return t, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		DeleteToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(tks, ref.Id)
			return nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		GetToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) (*core.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return tks[ref.Id], nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		ListTokens(gomock.Any()).
		DoAndReturn(func(_ context.Context) ([]*core.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			tokens := make([]*core.BootstrapToken, 0, len(tks))
			for _, t := range tks {
				tokens = append(tokens, t)
			}
			return tokens, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		UpdateToken(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference, mutator storage.MutatorFunc[*core.BootstrapToken]) (*core.BootstrapToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			token := tks[ref.Id]
			cloned := proto.Clone(token).(*core.BootstrapToken)
			mutator(cloned)
			if _, ok := tks[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			tks[ref.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()

	return mockTokenStore
}

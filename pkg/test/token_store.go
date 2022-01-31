package test

import (
	"context"
	"sync"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	mock_storage "github.com/kralicky/opni-monitoring/pkg/test/mock/storage"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
)

func NewTestTokenStore(ctx context.Context, ctrl *gomock.Controller) storage.TokenStore {
	mockTokenStore := mock_storage.NewMockTokenStore(ctrl)

	leaseStore := NewLeaseStore(ctx)
	tks := map[string]*tokens.Token{}
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
		CreateToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ttl time.Duration) error {
			mu.Lock()
			defer mu.Unlock()
			t := tokens.NewToken()
			lease := leaseStore.New(t.HexID(), ttl)
			t.Metadata.LeaseID = lease.ID
			t.Metadata.TTL = int64(ttl)
			tks[t.HexID()] = t
			return nil
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
		TokenExists(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := tks[ref.Id]; !ok {
				return false, nil
			}
			return true, nil
		}).
		AnyTimes()
	mockTokenStore.EXPECT().
		GetToken(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) (*tokens.Token, error) {
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
		DoAndReturn(func(_ context.Context) ([]*tokens.Token, error) {
			mu.Lock()
			defer mu.Unlock()
			tokens := make([]*tokens.Token, 0, len(tks))
			for _, t := range tks {
				tokens = append(tokens, t)
			}
			return tokens, nil
		}).
		AnyTimes()

	return mockTokenStore
}

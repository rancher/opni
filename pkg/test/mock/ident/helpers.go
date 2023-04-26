package mock_ident

import (
	"github.com/golang/mock/gomock"
	"github.com/rancher/opni/pkg/ident"
)

func NewTestIdentProvider(ctrl *gomock.Controller, id string) ident.Provider {
	mockIdent := NewMockProvider(ctrl)
	mockIdent.EXPECT().
		UniqueIdentifier(gomock.Any()).
		Return(id, nil).
		AnyTimes()
	return mockIdent
}

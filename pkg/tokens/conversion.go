package tokens

import (
	"encoding/hex"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func FromBootstrapToken(t *corev1.BootstrapToken) (*Token, error) {
	tokenID := t.GetTokenID()
	tokenSecret := t.GetSecret()
	token := &Token{
		ID:     make([]byte, hex.DecodedLen(len(tokenID))),
		Secret: make([]byte, hex.DecodedLen(len(tokenSecret))),
	}
	decodedID, err := hex.DecodeString(tokenID)
	if err != nil {
		return nil, err
	}
	decodedSecret, err := hex.DecodeString(tokenSecret)
	if err != nil {
		return nil, err
	}
	copy(token.ID, decodedID)
	copy(token.Secret, decodedSecret)
	return token, nil
}

func (t *Token) ToBootstrapToken() *corev1.BootstrapToken {
	return &corev1.BootstrapToken{
		TokenID: t.HexID(),
		Secret:  t.HexSecret(),
		Metadata: &corev1.BootstrapTokenMetadata{
			Labels: map[string]string{},
		},
	}
}

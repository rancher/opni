package management

import "github.com/kralicky/opni-gateway/pkg/tokens"

func (t *BootstrapToken) ToToken() *tokens.Token {
	tokenID := t.GetTokenID()
	tokenSecret := t.GetSecret()
	token := &tokens.Token{
		ID:     make([]byte, len(tokenID)),
		Secret: make([]byte, len(tokenSecret)),
		Metadata: tokens.TokenMeta{
			LeaseID: t.GetLeaseID(),
			TTL:     t.GetTTL(),
		},
	}
	copy(token.ID, tokenID)
	copy(token.Secret, tokenSecret)
	return token
}

func BootstrapTokenFromToken(token *tokens.Token) *BootstrapToken {
	t := &BootstrapToken{
		TokenID: make([]byte, len(token.ID)),
		Secret:  make([]byte, len(token.Secret)),
		LeaseID: token.Metadata.LeaseID,
		TTL:     token.Metadata.TTL,
	}
	copy(t.TokenID, token.ID)
	copy(t.Secret, token.Secret)
	return t
}

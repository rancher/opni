package management

import (
	"github.com/kralicky/opni-gateway/pkg/rbac"
	"github.com/kralicky/opni-gateway/pkg/tokens"
)

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

func NewBootstrapToken(token *tokens.Token) *BootstrapToken {
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

func (r *Role) ToRole() *rbac.Role {
	return &rbac.Role{
		Name:      r.GetName(),
		TenantIDs: r.GetTenantIDs(),
	}
}

func NewRole(role rbac.Role) *Role {
	return &Role{
		Name:      role.Name,
		TenantIDs: role.TenantIDs,
	}
}

func (r *RoleBinding) ToRoleBinding() *rbac.RoleBinding {
	return &rbac.RoleBinding{
		Name:     r.GetName(),
		RoleName: r.GetRoleName(),
		UserID:   r.GetUserID(),
	}
}

func NewRoleBinding(roleBinding rbac.RoleBinding) *RoleBinding {
	return &RoleBinding{
		Name:     roleBinding.Name,
		RoleName: roleBinding.RoleName,
		UserID:   roleBinding.UserID,
	}
}

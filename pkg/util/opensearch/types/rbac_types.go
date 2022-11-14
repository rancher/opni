package types

type RoleSpec struct {
	RoleName           string                  `json:"-"`
	ClusterPermissions []string                `json:"cluster_permissions,omitempty"`
	IndexPermissions   []IndexPermissionSpec   `json:"index_permissions,omitempty"`
	TenantPermissions  []TenantPermissionsSpec `json:"tenant_permissions,omitempty"`
}

type IndexPermissionSpec struct {
	IndexPatterns         []string `json:"index_patterns,omitempty"`
	DocumentLevelSecurity string   `json:"dls,omitempty"`
	FieldLevelSecurity    []string `json:"fls,omitempty"`
	AllowedActions        []string `json:"allowed_actions,omitempty"`
}

type TenantPermissionsSpec struct {
	TenantPatterns []string `json:"tenant_patterns,omitempty"`
	AllowedActions []string `json:"allowed_actions,omitempty"`
}

type UserSpec struct {
	UserName                string            `json:"-"`
	Password                string            `json:"password,omitempty"`
	Hash                    string            `json:"hash,omitempty"`
	OpendistroSecurityRoles []string          `json:"opendistro_security_roles,omitempty"`
	BackendRoles            []string          `json:"backend_roles,omitempty"`
	Attributes              map[string]string `json:"attributes,omitempty"`
}

type RoleMappingSpec struct {
	BackendRoles []string `json:"backend_roles,omitempty"`
	Hosts        []string `json:"hosts,omitempty"`
	Users        []string `json:"users,omitempty"`
}

type RoleMappingReponse map[string]RoleMappingSpec

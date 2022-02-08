package core

import "errors"

var (
	ErrReferenceRequiresID   = errors.New("reference requires ID to be set; this object is uniquely identified by ID")
	ErrReferenceRequiresName = errors.New("reference requires Name to be set; this object is uniquely identified by name")
)

type Referencer interface {
	Reference() *Reference
}

func (c *Cluster) Reference() *Reference {
	return &Reference{
		Id: c.Id,
	}
}

func (r *Role) Reference() *Reference {
	return &Reference{
		Name: r.Name,
	}
}

func (r *RoleBinding) Reference() *Reference {
	return &Reference{
		Name: r.Name,
	}
}

func (r *BootstrapToken) Reference() *Reference {
	return &Reference{
		Id: r.TokenID,
	}
}

func (r *RoleBinding) RoleReference() *Reference {
	return &Reference{
		Name: r.RoleName,
	}
}

func (r *Reference) Equal(other *Reference) bool {
	if r.Id == "" && other.Id == "" {
		return r.Name == other.Name
	}
	return r.Id == other.Id
}

func (r *Reference) CheckValidID() error {
	if r.Id == "" {
		return ErrReferenceRequiresID
	}
	return nil
}

func (r *Reference) CheckValidName() error {
	if r.Name == "" {
		return ErrReferenceRequiresName
	}
	return nil
}

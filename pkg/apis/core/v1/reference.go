package v1

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
		Id: r.Id,
	}
}

func (r *RoleBinding) Reference() *Reference {
	return &Reference{
		Id: r.Id,
	}
}

func (r *BootstrapToken) Reference() *Reference {
	return &Reference{
		Id: r.TokenID,
	}
}

func (r *RoleBinding) RoleReference() *Reference {
	return &Reference{
		Id: r.RoleId,
	}
}

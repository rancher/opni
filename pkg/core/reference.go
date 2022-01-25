package core

import "errors"

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
		return errors.New("reference has empty id")
	}
	return nil
}

func (r *Reference) CheckValidName() error {
	if r.Name == "" {
		return errors.New("reference has empty name")
	}
	return nil
}

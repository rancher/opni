package config

import (
	"errors"

	"github.com/rancher/opni/pkg/config/meta"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Lifecycler interface {
	GetObjectList() (meta.ObjectList, error)
	UpdateObjectList(objects meta.ObjectList) error
	ReloadC() (chan struct{}, error)
}

type lifecycler struct {
	objects meta.ObjectList
	reloadC chan struct{}
}

func NewLifecycler(objects meta.ObjectList) Lifecycler {
	return &lifecycler{
		objects: objects,
		reloadC: make(chan struct{}),
	}
}

func (l *lifecycler) ReloadC() (chan struct{}, error) {
	return l.reloadC, nil
}

func (l *lifecycler) GetObjectList() (meta.ObjectList, error) {
	return l.objects, nil
}

func (l *lifecycler) UpdateObjectList(objects meta.ObjectList) error {
	l.objects = objects
	select {
	case l.reloadC <- struct{}{}:
	default:
		return errors.New("no reload handler available")
	}
	return nil
}

// default no-op lifecycler with limited functionality
type unavailableLifecycler struct {
	objects meta.ObjectList
}

func NewUnavailableLifecycler(objects meta.ObjectList) Lifecycler {
	return &unavailableLifecycler{
		objects: objects,
	}
}

func (l *unavailableLifecycler) ReloadC() (chan struct{}, error) {
	return nil, status.Error(codes.Unavailable, "lifecycler not available")
}

func (l *unavailableLifecycler) GetObjectList() (meta.ObjectList, error) {
	return l.objects, nil
}
func (l *unavailableLifecycler) UpdateObjectList(_ meta.ObjectList) error {
	return status.Error(codes.Unavailable, "lifecycler not available")
}

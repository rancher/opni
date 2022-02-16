package config

import "github.com/rancher/opni-monitoring/pkg/config/meta"

type Lifecycler interface {
	GetObjectList() (meta.ObjectList, error)
	UpdateObjectList(objects meta.ObjectList) error
	ReloadC() (chan struct{}, error)
}

type lifecycler struct {
	objects meta.ObjectList
	reloadC chan struct{}
}

func NewLifecycler(objects meta.ObjectList) *lifecycler {
	return &lifecycler{
		objects: objects,
		reloadC: make(chan struct{}, 1),
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
	l.reloadC <- struct{}{}
	return nil
}

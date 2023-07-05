package jetstream

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	"github.com/samber/lo"
)

type ConditionGroupManager[T interfaces.AlertingSecret] struct {
	kv          nats.KeyValue
	defaultPath string

	groupPathFunc func(groupId string) string
	groupPathRe   *regexp.Regexp
}

func NewConditionGroupManager[T interfaces.AlertingSecret](
	store nats.KeyValue,
	defaultPath string,
	groupPathFunc func(groupId string) string,
	groupRe *regexp.Regexp,
) *ConditionGroupManager[T] {
	return &ConditionGroupManager[T]{
		kv:            store,
		defaultPath:   defaultPath,
		groupPathFunc: groupPathFunc,
		groupPathRe:   groupRe,
	}
}

func keys(kv nats.KeyValue) ([]string, error) {
	keys, err := kv.Keys()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}
	return keys, nil
}

func (m *ConditionGroupManager[T]) Group(groupId string) spec.AlertingSecretStorage[T] {
	if groupId == "" {
		return NewJetStreamAlertingStorage[T](
			m.kv,
			m.defaultPath,
		)
	}
	return NewJetStreamAlertingStorage[T](
		m.kv,
		m.groupPathFunc(groupId),
	)
}

func (m *ConditionGroupManager[T]) ListGroups(_ context.Context) ([]string, error) {
	allKeys, err := keys(m.kv)
	if err != nil {
		return nil, err
	}
	groups := map[string]struct{}{}
	for _, key := range allKeys {
		if groupId, err := m.extractGroupId(key); err == nil {
			groups[groupId] = struct{}{}
		}
	}
	resGroups := append(lo.Keys(groups), "")
	sort.Strings(resGroups)
	return resGroups, nil
}

func (m *ConditionGroupManager[T]) extractGroupId(key string) (string, error) {
	matches := m.groupPathRe.FindStringSubmatch(key)
	if len(matches) != 2 {
		return "", fmt.Errorf("key %s does not match group path regex %s", key, m.groupPathRe.String())
	}
	return matches[1], nil
}

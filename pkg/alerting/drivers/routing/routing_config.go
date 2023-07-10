package routing

import (
	"time"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"
)

var (
	DefaultConfig = Config{
		GlobalConfig: GlobalConfig{
			GroupWait:      lo.ToPtr(model.Duration(60 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		SubtreeConfig: SubtreeConfig{
			GroupWait:      lo.ToPtr(model.Duration(60 * time.Second)),
			RepeatInterval: lo.ToPtr(model.Duration(5 * time.Hour)),
		},
		FinalizerConfig: FinalizerConfig{
			InitialDelay:       time.Second * 10,
			ThrottlingDuration: time.Minute * 1,
			RepeatInterval:     time.Hour * 5,
		},
		NotificationConfg: NotificationConfg{},
	}
)

type Config struct {
	GlobalConfig      `yaml:"global", json:"global"`
	SubtreeConfig     `yaml:"subtree", json:"subtree"`
	FinalizerConfig   `yaml:"finalizer", json:"finalizer"`
	NotificationConfg `yaml:"notification", json:"notification"`
}

type FinalizerConfig struct {
	InitialDelay       time.Duration `yaml:"initialDelay", json:"initialDelay"`
	ThrottlingDuration time.Duration `yaml:"throttlingDuration", json:"throttlingDuration"`
	RepeatInterval     time.Duration `yaml:"repeatInterval", json:"repeatInterval"`
}

type GlobalConfig struct {
	GroupWait      *model.Duration `yaml:"groupWait", json:"groupWait"`
	RepeatInterval *model.Duration `yaml:"repeatInterval", json:"repeatInterval"`
}

type SubtreeConfig struct {
	GroupWait      *model.Duration `yaml:"groupWait", json:"groupWait"`
	RepeatInterval *model.Duration `yaml:"repeatInterval", json:"repeatInterval"`
}

type NotificationConfg struct {
	//TODO: add notification config
}

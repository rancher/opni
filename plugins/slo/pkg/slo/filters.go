package slo

import (
	"embed"
	_ "embed"
	"fmt"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io/fs"
	"path/filepath"
	"regexp"
)

//go:embed metricgroups/*.yaml
var MetricGroups embed.FS

//go:embed servicegroups/*.yaml
var ServiceGroups embed.FS

// map of directory names to their embed.FS
var EnabledFilters = map[string]embed.FS{"metricgroups": MetricGroups, "servicegroups": ServiceGroups}
var filters = constructFilters(logger.NewPluginLogger().Named("slo"))

// Regexp adds unmarshalling from json for regexp.Regexp
type Regexp struct {
	*regexp.Regexp
}

// UnmarshalText unmarshals json into a regexp.Regexp
func (r *Regexp) UnmarshalText(b []byte) error {
	regex, err := regexp.Compile(string(b))
	if err != nil {
		return err
	}

	r.Regexp = regex

	return nil
}

// MarshalText marshals regexp.Regexp as string
func (r *Regexp) MarshalText() ([]byte, error) {
	if r.Regexp != nil {
		return []byte(r.Regexp.String()), nil
	}

	return nil, nil
}

type Filter struct {
	Name    string   `yaml:"name"`
	Filters []Regexp `yaml:"filters"`
	Ignore  []Regexp `yaml:"ignore"`
}

func GetGroupConfigsFromEmbed(lg *zap.SugaredLogger, dirName string, dir embed.FS) []Filter {
	lg = lg.With("plugin", "slo", "phase", "init")
	var res []Filter
	fsys := fs.FS(dir)
	yamlFs, err := fs.Sub(fsys, dirName)
	if err != nil {
		lg.Error(err)
		lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
	}
	err = fs.WalkDir(yamlFs, ".", func(pathStr string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			fBytes, err := fs.ReadFile(dir, filepath.Join(dirName, d.Name()))
			if err != nil {
				lg.Error(err)
				lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
				panic(err)
			}
			var f Filter
			err = yaml.Unmarshal(fBytes, &f)
			if err != nil {
				lg.Error(err)
				lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
			}
			res = append(res, f)
		}
		return nil
	})
	if err != nil {
		lg.Error(err)
		lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
	}
	return res
}

func constructFilters(lg *zap.SugaredLogger) []Filter {
	res := []Filter{}
	for dirName, embedFs := range EnabledFilters {
		filters := GetGroupConfigsFromEmbed(lg, dirName, embedFs)
		res = append(res, filters...)
	}
	return res
}

// Map metric -> group -> score
func scoredLabels(labels *cortexadmin.MetricLabels) *sloapi.EventGroupList {
	res := map[*cortexadmin.LabelSet]map[string]int{}
	for _, labelObj := range labels.Items {
		eventName := labelObj.GetName()
		for _, groupname := range filters {
			for _, filter := range groupname.Filters {
				if filter.MatchString(eventName) {
					if _, ok := res[labelObj]; !ok {
						res[labelObj] = map[string]int{}
					}
					res[labelObj][groupname.Name] += 1
				}
			}
			for _, filter := range groupname.Ignore {
				if filter.MatchString(eventName) {
					if _, ok := res[labelObj]; !ok {
						res[labelObj] = map[string]int{}
					}
					res[labelObj][groupname.Name] -= 1
				}
			}
		}
	}
	groupedEvents := &sloapi.EventGroupList{
		GroupNameToEvent: map[string]*sloapi.EventList{},
	}
	for event, vals := range res {
		groupName := ""
		maxScore := 0
		for group, score := range vals {
			if score > maxScore {
				maxScore = score
				groupName = group
			}
		}
		if maxScore <= 0 {
			groupName = "other events"
		}
		if _, ok := groupedEvents.GroupNameToEvent[groupName]; !ok {
			groupedEvents.GroupNameToEvent[groupName] = &sloapi.EventList{}
		}
		groupedEvents.GroupNameToEvent[groupName].Items = append(groupedEvents.GroupNameToEvent[groupName].Items, &sloapi.Event{
			Key:  event.GetName(),
			Vals: event.GetItems(),
		})
	}
	return groupedEvents
}

func ApplyFiltersToCortexEvents(labels *cortexadmin.MetricLabels) (*sloapi.EventGroupList, error) {
	return scoredLabels(labels), nil
}

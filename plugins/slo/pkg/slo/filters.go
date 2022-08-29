package slo

import (
	"embed"
	_ "embed"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path"
	"regexp"
)

//go:embed metricgroups/*.yaml
var MetricGroups embed.FS

//go:embed servicegroups/*.yaml
var ServiceGroups embed.FS

// map of directory names to their embed.FS
var EnabledFilters = map[string]embed.FS{"metricgroups": MetricGroups, "servicegroups": ServiceGroups}
var filters = constructFilters()

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

func GetGroupConfigsFromEmbed(dirName string, dir embed.FS) []Filter {
	dirFs, err := dir.ReadDir(dirName)
	if err != nil {
		panic(err)
	}
	return parseToFilter(dirName, dirFs)
}

func parseToFilter(dirName string, dirFs []fs.DirEntry) []Filter {
	var res []Filter
	for _, file := range dirFs {
		fBytes, err := os.ReadFile(path.Join(dirName, file.Name()))
		if err != nil {
			panic(err)
		}
		var f Filter
		err = yaml.Unmarshal(fBytes, &f)
		if err != nil {
			panic(err)
		}
		res = append(res, f)

	}
	return res
}

func constructFilters() []Filter {
	res := []Filter{}
	for dirName, embedFs := range EnabledFilters {
		filters := GetGroupConfigsFromEmbed(dirName, embedFs)
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

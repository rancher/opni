package slo

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/apis/slo"
	"gopkg.in/yaml.v3"
	"log/slog"
)

//go:embed metricgroups/*.yaml
var MetricGroups embed.FS

//go:embed servicegroups/*.yaml
var ServiceGroups embed.FS

// map of directory names to their embed.FS
var EnabledFilters = map[string]embed.FS{"metricgroups": MetricGroups, "servicegroups": ServiceGroups}
var filters = constructFilters(logger.NewPluginLogger().WithGroup("slo"))

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
	Name    string        `yaml:"name"`
	Filters []FilterValue `yaml:"filters"`
	Ignore  []FilterValue `yaml:"ignore"`
}

type FilterValue struct {
	Value Regexp `yaml:"value"`
	Score int    `yaml:"score"`
}

func GetGroupConfigsFromEmbed(lg *slog.Logger, dirName string, dir embed.FS) []Filter {
	lg = lg.With("plugin", "slo", "phase", "init")
	var res []Filter
	fsys := fs.FS(dir)
	yamlFs, err := fs.Sub(fsys, dirName)
	if err != nil {
		lg.Error("error", logger.Err(err))
		lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
	}
	err = fs.WalkDir(yamlFs, ".", func(pathStr string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			fBytes, err := fs.ReadFile(dir, filepath.Join(dirName, d.Name()))
			if err != nil {
				lg.Error("error", logger.Err(err))
				lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
				panic(err)
			}
			var f Filter
			err = yaml.Unmarshal(fBytes, &f)
			if err != nil {
				lg.Error("error", logger.Err(err))
				lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
			}
			res = append(res, f)
		}
		return nil
	})
	if err != nil {
		lg.Error("error", logger.Err(err))
		lg.Debug(fmt.Sprintf("Debug error : yamlFs is %s", yamlFs))
	}
	return res
}

func constructFilters(lg *slog.Logger) []Filter {
	res := []Filter{}
	for dirName, embedFs := range EnabledFilters {
		filters := GetGroupConfigsFromEmbed(lg, dirName, embedFs)
		res = append(res, filters...)
	}
	return res
}

// Map metric -> group -> score
func scoredLabels(seriesInfo *cortexadmin.SeriesInfoList) *sloapi.MetricGroupList {
	res := map[*cortexadmin.SeriesInfo]map[string]int{}

	for _, series := range seriesInfo.GetItems() {
		series.GetSeriesName()
		for _, groupname := range filters {
			for _, matchFilter := range groupname.Filters {
				if matchFilter.Value.MatchString(series.GetSeriesName()) {
					if _, ok := res[series]; !ok {
						res[series] = map[string]int{}
					}
					res[series][groupname.Name] += matchFilter.Score
				}
			}

			for _, ignoreFilter := range groupname.Ignore {
				if ignoreFilter.Value.MatchString(series.GetSeriesName()) {
					if _, ok := res[series]; !ok {
						res[series] = map[string]int{}
					}
					res[series][groupname.Name] -= ignoreFilter.Score
				}
			}
		}
	}
	groupedMetrics := sloapi.MetricGroupList{
		GroupNameToMetrics: map[string]*sloapi.MetricList{},
	}
	for series, vals := range res {
		groupName := ""
		maxScore := 0
		for group, score := range vals {
			if score > maxScore {
				maxScore = score
				groupName = group
			}
		}
		if maxScore <= 0 {
			groupName = "other metrics"
		}
		if _, ok := groupedMetrics.GroupNameToMetrics[groupName]; !ok {
			groupedMetrics.GroupNameToMetrics[groupName] = &sloapi.MetricList{}
		}
		groupedMetrics.GroupNameToMetrics[groupName].Items = append(groupedMetrics.GroupNameToMetrics[groupName].Items, &sloapi.Metric{
			Id: series.GetSeriesName(),
			Metadata: &sloapi.MetricMetadata{
				Description: series.Metadata.GetDescription(),
				Unit:        series.Metadata.GetUnit(),
				Type:        series.Metadata.GetType(),
			},
		})
	}
	return &groupedMetrics
}

func ApplyFiltersToCortexEvents(seriesInfo *cortexadmin.SeriesInfoList) (*sloapi.MetricGroupList, error) {
	return scoredLabels(seriesInfo), nil
}

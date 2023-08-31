package metrics

import (
	"embed"
	"io/fs"
	"path/filepath"
	"regexp"
	"sync"

	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed metricgroups/*.yaml
var metricGroups embed.FS

//go:embed servicegroups/*.yaml
var serviceGroups embed.FS

// map of directory names to their embed.FS
var EnabledFilters = map[string]embed.FS{"metricgroups": metricGroups, "servicegroups": serviceGroups}

type MetricsBackendProvider struct {
	lg          *zap.SugaredLogger
	filters     []Filter
	adminClient future.Future[cortexadmin.CortexAdminClient]
}

func NewProvider(
	lg *zap.SugaredLogger,
) *MetricsBackendProvider {
	return &MetricsBackendProvider{
		lg:          lg,
		adminClient: future.New[cortexadmin.CortexAdminClient](),
	}
}

func (m *MetricsBackendProvider) Initialize(cl cortexadmin.CortexAdminClient) {
	m.adminClient.Set(cl)
	m.filters = m.constructFilters()
}

func ApplyFiltersToMetricEvents(seriesInfo *cortexadmin.SeriesInfoList, filters []Filter) *slov1.MetricGroupList {
	return scoredLabels(seriesInfo, filters)
}

func GetGroupConfigsFromEmbed(dirName string, dir embed.FS) []Filter {
	var res []Filter
	var mu sync.Mutex
	fsys := fs.FS(dir)
	yamlFs, err := fs.Sub(fsys, dirName)
	if err != nil {
		return []Filter{}
	}
	fs.WalkDir(yamlFs, ".", func(pathStr string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			fBytes, err := fs.ReadFile(dir, filepath.Join(dirName, pathStr))
			if err != nil {
				panic(err)
			}
			var f Filter
			err = yaml.Unmarshal(fBytes, &f)
			if err == nil {
				mu.Lock()
				res = append(res, f)
				mu.Unlock()
			}
		}
		return nil
	})
	return res
}

func scoredLabels(seriesInfo *cortexadmin.SeriesInfoList, filters []Filter) *slov1.MetricGroupList {

	seriesNameToScoreGroup := map[string]map[string]int{}

	seriesNameToSeries := lo.Associate(seriesInfo.GetItems(), func(series *cortexadmin.SeriesInfo) (string, *cortexadmin.SeriesInfo) {
		return series.GetSeriesName(), series
	})

	for _, series := range seriesInfo.GetItems() {
		seriesName := series.GetSeriesName()
		for _, groupname := range filters {
			for _, matchFilter := range groupname.Filters {
				if matchFilter.Value.MatchString(series.GetSeriesName()) {
					if _, ok := seriesNameToScoreGroup[seriesName]; !ok {
						seriesNameToScoreGroup[seriesName] = map[string]int{}
					}
					seriesNameToScoreGroup[seriesName][groupname.Name] += matchFilter.Score
				}
			}

			for _, ignoreFilter := range groupname.Ignore {
				if ignoreFilter.Value.MatchString(series.GetSeriesName()) {
					if _, ok := seriesNameToScoreGroup[seriesName]; !ok {
						seriesNameToScoreGroup[seriesName] = map[string]int{}
					}
					seriesNameToScoreGroup[seriesName][groupname.Name] -= ignoreFilter.Score
				}
			}
		}
	}
	groupedMetrics := &slov1.MetricGroupList{
		GroupNameToMetrics: map[string]*slov1.MetricList{},
	}
	for seriesName, data := range seriesNameToSeries {
		groupName := ""
		maxScore := 0

		if vals, ok := seriesNameToScoreGroup[seriesName]; ok {
			for group, score := range vals {
				if score > maxScore {
					maxScore = score
					groupName = group
				}
			}
		}

		if maxScore <= 0 {
			groupName = "other metrics"
		}
		if _, ok := groupedMetrics.GroupNameToMetrics[groupName]; !ok {
			groupedMetrics.GroupNameToMetrics[groupName] = &slov1.MetricList{}
		}
		groupedMetrics.GroupNameToMetrics[groupName].Items = append(groupedMetrics.GroupNameToMetrics[groupName].Items, &slov1.Metric{
			Id: seriesName,
			Metadata: &slov1.MetricMetadata{
				Description: data.Metadata.GetDescription(),
				Unit:        data.Metadata.GetUnit(),
				Type:        data.Metadata.GetType(),
			},
		})
	}
	return groupedMetrics
}

func (m *MetricsBackendProvider) constructFilters() []Filter {
	res := []Filter{}
	for dirName, embedFs := range EnabledFilters {
		filters := GetGroupConfigsFromEmbed(dirName, embedFs)
		res = append(res, filters...)
	}
	return res
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

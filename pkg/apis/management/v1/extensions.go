package v1

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
)

func (gc *GatewayConfig) YAMLDocuments() [][]byte {
	docs := [][]byte{}
	for _, doc := range gc.Documents {
		docs = append(docs, doc.Yaml)
	}
	return docs
}

func (cl *CapabilityList) Names() []string {
	names := []string{}
	for _, c := range cl.Items {
		names = append(names, c.Details.Name)
	}
	return names
}

func (m *ListClustersRequest) CacheKey() string {
	if m.MatchLabels == nil && m.MatchOptions == 0 {
		return "all"
	}
	if len(m.MatchLabels.MatchExpressions) == 0 &&
		len(m.MatchLabels.MatchLabels) == 0 &&
		m.MatchOptions == 0 {
		return "all"
	}
	key := fmt.Sprintf("%d", m.MatchOptions)
	for _, l := range m.MatchLabels.GetMatchExpressions() {
		key += l.Key + l.Operator + strings.Join(l.Values, "")
	}
	matchLabelKeys := lo.Keys(m.MatchLabels.GetMatchLabels())
	sort.Strings(matchLabelKeys)
	for _, mKey := range matchLabelKeys {
		key += mKey + m.MatchLabels.MatchLabels[mKey]
	}
	return key
}

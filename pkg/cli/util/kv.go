package util

import (
	"fmt"
	"strings"
)

// Parses strings of the form "key=value" into a map[string]string.
func ParseKeyValuePairs(pairs []string) (map[string]string, error) {
	m := map[string]string{}
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid syntax: %q", pair)
		}
		m[kv[0]] = kv[1]
	}
	return m, nil
}

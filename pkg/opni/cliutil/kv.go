package cliutil

import (
	"fmt"
	"strings"
)

// Parses strings of the form "key=value" into a map[string]string.
func ParseKeyValuePairs(pairs []string) (map[string]string, error) {
	m := map[string]string{}
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		switch len(kv) {
		case 2:
			m[kv[0]] = kv[1]
		case 1:
			if strings.HasSuffix(kv[0], "-") {
				m[strings.TrimSuffix(kv[0], "-")] = "-"
			}
		default:
			return nil, fmt.Errorf("invalid syntax: %q", pair)
		}
	}
	return m, nil
}

func JoinKeyValuePairs(pairs map[string]string) []string {
	kvs := make([]string, 0, len(pairs))
	for k, v := range pairs {
		kvs = append(kvs, k+"="+v)
	}
	return kvs
}

package util

import "strings"

// Implements rbac.Codec
type DelimiterCodec struct {
	key       string
	delimiter string
}

func (d DelimiterCodec) Key() string {
	return d.key
}

func (d DelimiterCodec) Encode(ids []string) string {
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		filtered = append(filtered, strings.TrimSpace(id))
	}
	return strings.Join(filtered, d.delimiter)
}

func (d DelimiterCodec) Decode(s string) []string {
	parts := strings.Split(s, d.delimiter)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

func NewDelimiterCodec(key string, delimiter string) DelimiterCodec {
	return DelimiterCodec{
		key:       key,
		delimiter: delimiter,
	}
}

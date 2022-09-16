package annotations

const (
	AgentVersion = "opni.io/agent-version"
	Version1     = "v1"
	Version2     = "v2"
)

func KeyValuePairs(kv map[string]string) []any {
	pairs := make([]any, 0, 2*len(kv))
	for k, v := range kv {
		pairs = append(pairs, k, v)
	}
	return pairs
}

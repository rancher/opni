package rbac

type HeaderCodec interface {
	Key() string
	Encode(ids []string) string
	Decode(s string) []string
}

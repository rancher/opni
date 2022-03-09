package rbac

type Codec interface {
	Key() string
	Encode(ids []string) string
	Decode(s string) []string
}

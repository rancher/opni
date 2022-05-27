package v1

import _ "embed"

//go:embed management.swagger.json
var managementSwaggerJson []byte

func OpenAPISpec() []byte {
	buf := make([]byte, len(managementSwaggerJson))
	copy(buf, managementSwaggerJson)
	return buf
}

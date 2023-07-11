package node

import (
	"google.golang.org/grpc/metadata"
)

const defaultConfigFlag = "is-default-config"

func IsDefaultConfig(trailer metadata.MD) bool {
	if len(trailer[defaultConfigFlag]) > 0 {
		return trailer[defaultConfigFlag][0] == "true"
	}
	return false
}

func DefaultConfigMetadata() metadata.MD {
	return metadata.Pairs(defaultConfigFlag, "true")
}

package staging

import _ "embed"

//go:generate ./generate.sh

//go:embed staging_autogen.yaml
var StagingAutogenYaml string

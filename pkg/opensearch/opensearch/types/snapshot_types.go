package types

type RepositoryType string

const (
	RepositoryTypeS3         RepositoryType = "s3"
	RepositoryTypeFileSystem RepositoryType = "fs"
)

type RepositoryRequest struct {
	Type     RepositoryType     `json:"type"`
	Settings RepositorySettings `json:"settings"`
}

type RepositorySettings struct {
	*S3Settings         `json:",inline,omitempty"`
	*FileSystemSettings `json:",inline,omitempty"`
}

type S3Settings struct {
	Bucket    string `json:"bucket"`
	Path      string `json:"base_path"`
	ReadOnly  bool   `json:"readonly"`
	ChunkSize string `json:"chunk_size,omitempty"`
}

type FileSystemSettings struct {
	Location string `json:"location"`
	Readonly bool   `json:"readonly"`
}

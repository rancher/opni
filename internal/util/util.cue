package util

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/docker"
	"encoding/json"
)

// scratch docker image
Scratch: docker.#Image & {
	rootfs: dagger.#Scratch
	config: {}
}

#FetchJSON: {
	source: string

	_fetch: core.#HTTPFetch & {
		"source": source
		dest:   "/response"
	}
	_read: core.#ReadFile & {
		input: _fetch.output
		path:  "/response"
	}

	output: json.Unmarshal(_read.contents)
}

// #MergeSubdirs: {
// 	input: dagger.#FS
// 	paths: [...string]

// 	core.#Merge & {
// 		"inputs": [
// 			for _, subdir in [
// 				for _, path in paths {
// 					core.#Subdir & {
// 						"input":  input
// 						"path":   path
// 					}
// 				},
// 			] {
// 				subdir.output
// 			},
// 		]
// 	}
// }

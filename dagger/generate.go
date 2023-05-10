package main

import (
	"dagger.io/dagger"
)

func (b *builder) generate(pipeline *dagger.Container) *dagger.Container {
	proto := pipeline.WithDirectory(b.workdir, b.generateProto(pipeline))

	b.generateMocks(proto)

	controllers := b.generateControllers(proto)

	return pipeline.WithDirectory(b.workdir, controllers)
}

func (b *builder) generateProto(pipeline *dagger.Container) *dagger.Directory {
	protoSources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
		Include: []string{
			"magefiles/generate_proto.go",
			"go.mod",
			"go.sum",
			"internal/cli",
			"**/*.proto",
			"**/*.pb.*",
		},
	})

	preGen := pipeline.WithMountedDirectory(b.workdir, protoSources)
	postGen := preGen.WithExec([]string{"mage", "-v", "protobufgo"})

	generatedFiles := preGen.Directory(b.workdir).
		Diff(postGen.Directory(b.workdir))

	b.ExportToRoot(generatedFiles)

	return generatedFiles
}

func (b *builder) generateMocks(pipeline *dagger.Container) *dagger.Directory {
	// todo: only generate source files newer than the generated ones
	sources := []string{
		"pkg/test/mock/mockgen.yaml",
		"magefiles/generate_mocks.go",
		"go.mod",
		"go.sum",
	}
	for _, m := range b.hostInfo.MockgenConfig.Mocks {
		if m.Source == "" {
			continue
		}
		sources = append(sources, m.Source, m.Dest)
	}

	mockSources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
		Include: sources,
	})

	preGen := pipeline.WithMountedDirectory(b.workdir, mockSources)
	postGen := preGen.WithExec([]string{"mage", "-v", "mockgen"})

	generatedFiles := preGen.Directory(b.workdir).
		Diff(postGen.Directory(b.workdir))

	b.ExportToRoot(generatedFiles)

	return generatedFiles
}

func (b *builder) generateControllers(pipeline *dagger.Container) *dagger.Directory {
	controllerSources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
		Include: []string{
			"magefiles/generate_controllers.go",
			"go.mod",
			"go.sum",
			"apis/",
			"config/",
			"pkg/",
			"internal/",
		},
	})

	preGen := pipeline.WithMountedDirectory(b.workdir, controllerSources)
	postGen := preGen.WithExec([]string{"mage", "-v", "controllergen"})
	generatedFiles := preGen.Directory(b.workdir).
		Diff(postGen.Directory(b.workdir))

	b.ExportToRoot(generatedFiles)

	return generatedFiles
}

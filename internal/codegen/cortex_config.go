package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cortexproject/cortex/pkg/compactor"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/querier"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/validation"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/builder"
	"github.com/kralicky/protols/pkg/format"
	"github.com/kralicky/protols/pkg/lsp"
	"github.com/kralicky/protols/pkg/sources"
	"github.com/rancher/opni/internal/codegen/cli"
	"github.com/rancher/opni/internal/codegen/descriptors"
	"github.com/samber/lo"
	"golang.org/x/tools/gopls/pkg/lsp/protocol"
	"golang.org/x/tools/gopls/pkg/span"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func GenCortexConfig() error {
	newProtos := map[string]*desc.FileDescriptor{}
	{
		path := "github.com/rancher/opni/internal/cortex/config/storage/storage.proto"
		contents, err := generate[bucket.Config](path)
		if err != nil {
			return err
		}
		newProtos[path] = contents
	}
	{
		path := "github.com/rancher/opni/internal/cortex/config/validation/limits.proto"
		contents, err := generate[validation.Limits](path)
		if err != nil {
			return err
		}
		newProtos[path] = contents
	}
	{
		path := "github.com/rancher/opni/internal/cortex/config/runtimeconfig/runtimeconfig.proto"
		contents, err := generate[cortex.RuntimeConfigValues](path)
		if err != nil {
			return err
		}
		newProtos[path] = contents
	}
	{
		path := "github.com/rancher/opni/internal/cortex/config/compactor/compactor.proto"
		contents, err := generate[compactor.Config](path)
		if err != nil {
			return err
		}
		newProtos[path] = contents
	}
	{
		path := "github.com/rancher/opni/internal/cortex/config/querier/querier.proto"
		contents, err := generate[querier.Config](path,
			func(rf reflect.StructField) bool {
				if rf.Name == "StoreGatewayAddresses" || rf.Name == "StoreGatewayClient" {
					return true
				}
				return false
			},
		)
		if err != nil {
			return err
		}
		newProtos[path] = contents
	}

	newProtoDescriptors := []protoreflect.FileDescriptor{}

	existingFiles := sources.SearchDirs("./internal")
	if len(existingFiles) > 0 {
		wd, _ := os.Getwd()
		cache := lsp.NewCache(protocol.WorkspaceFolder{
			URI: string(span.URIFromPath(wd)),
		})
		cache.LoadFiles(existingFiles)
		for path, fd := range newProtos {
			existing, err := cache.FindResultByPath(path)
			if err != nil {
				// no previous version of this file
				if !os.IsNotExist(err) {
					return err
				}
				continue
			}
			newFd := fd.UnwrapFile()
			oldFd := existing
			newFdProto := fd.AsFileDescriptorProto()
			mergeFileDescriptors(newFdProto, newFd, oldFd)
			newFd, err = protodesc.NewFile(newFdProto, cache.XGetResolver())
			if err != nil {
				return err
			}
			newProtoDescriptors = append(newProtoDescriptors, newFd)
		}
	}

	for _, fd := range newProtoDescriptors {
		name := fd.Path()
		rootDir := strings.TrimPrefix(filepath.Dir(name), "github.com/rancher/opni/")
		fullPath := filepath.Join(rootDir, filepath.Base(name))
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
		f, err := os.Create(fullPath)
		if err != nil {
			return err
		}
		err = format.PrintAndFormatFileDescriptor(fd, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// from cortex doc-generator
func parseDocTag(f reflect.StructField) map[string]string {
	cfg := map[string]string{}
	tag := f.Tag.Get("doc")

	if tag == "" {
		return cfg
	}

	for _, entry := range strings.Split(tag, "|") {
		parts := strings.SplitN(entry, "=", 2)

		switch len(parts) {
		case 1:
			cfg[parts[0]] = ""
		case 2:
			cfg[parts[0]] = parts[1]
		}
	}

	return cfg
}

func editFlagOptions(rf reflect.StructField) func(*cli.FlagOptions) {
	docInfo := parseDocTag(rf)
	if _, ok := docInfo["nocli"]; ok {
		return func(fo *cli.FlagOptions) {
			fo.Skip = true
		}
	} else if rf.Name == "TenantLimits" {
		return func(fo *cli.FlagOptions) {
			fo.Skip = true
		}
	} else if rf.Type == reflect.TypeOf(flagext.Secret{}) || isSpecialCaseSecretFieldName(rf.Name) {
		return func(fo *cli.FlagOptions) {
			fo.Secret = true
		}
	}
	return nil
}

func isSpecialCaseSecretFieldName(name string) bool {
	// a couple fields don't have the flagext.Secret type, so we need to
	// check for them specifically
	return name == "MSIResource" || // AzureConfig
		name == "KMSEncryptionContext" // S3SSEConfig
}

func editFieldComment(rf reflect.StructField, in *string) {
	docInfo := parseDocTag(rf)
	if doc, ok := docInfo["description"]; ok {
		*in = doc
	}
}

var cortexTypesToSkip = map[reflect.Type]bool{
	reflect.TypeOf(compactor.RingConfig{}): true,
}

var customFieldTypes = map[reflect.Type]func() *builder.FieldType{
	reflect.TypeOf(flagext.Secret{}): func() *builder.FieldType {
		return builder.FieldTypeString()
	},
}

func generate[T any](destFilename string, skipFunc ...func(rf reflect.StructField) bool) (*desc.FileDescriptor, error) {
	messages := descriptors.BuildMessage[T](descriptors.BuilderOptions{
		FieldNameFromTags:       []string{"json", "yaml"},
		EditFlagOptions:         editFlagOptions,
		EditFieldComment:        editFieldComment,
		AllScalarFieldsOptional: true,
		SkipFieldFunc: func(rf reflect.StructField) bool {
			if cortexTypesToSkip[rf.Type] {
				return true
			}
			if strings.HasPrefix(rf.Name, "Sharding") {
				return true
			}
			if strings.HasSuffix(rf.Name, "Dir") || strings.HasSuffix(rf.Name, "Directory") {
				return true
			}
			if _, ok := parseDocTag(rf)["hidden"]; ok {
				return true
			}
			if len(skipFunc) > 0 {
				return skipFunc[0](rf)
			}
			return false
		},
		CustomFieldTypes: customFieldTypes,
	})
	cliImport, _ := desc.WrapFile(cli.File_github_com_rancher_opni_internal_codegen_cli_cli_proto)
	f := builder.NewFile(destFilename).
		SetProto3(true).
		SetPackageName(filepath.Base(filepath.Dir(destFilename))).
		SetOptions(&descriptorpb.FileOptions{
			GoPackage: lo.ToPtr(filepath.Dir(destFilename)),
		}).
		SetSyntaxComments(builder.Comments{
			LeadingComment: `
Code generated by internal/codegen. You may edit parts of this file.
Field numbers and custom options will be preserved for matching field names.
All other modifications will be lost.
`[1:],
		}).
		AddImportedDependency(cliImport)
	proto.SetExtension(f.Options, cli.E_Generator, &cli.GeneratorOptions{
		Generate:         true,
		GenerateDeepcopy: true,
	})
	for _, m := range messages {
		f.AddMessage(m)
	}
	fd, err := f.Build()
	if err != nil {
		return nil, err
	}
	var t T
	tType := reflect.TypeOf(t)
	mainMsgDesc := fd.FindMessage(filepath.Base(filepath.Dir(destFilename)) + "." + tType.Name())
	if mainMsgDesc == nil {
		panic("bug: main message not found")
	}
	customFieldTypes[tType] = func() *builder.FieldType {
		return builder.FieldTypeImportedMessage(mainMsgDesc)
	}
	return fd, nil
}

func mergeFileDescriptors(targetpb *descriptorpb.FileDescriptorProto, target, existing protoreflect.FileDescriptor) {
	// diff the new and old message fields to ensure existing field numbers don't change

	// important: we only build messages in this generator right now. if other
	// top-level elements are added, this will need to be updated

	msgs := target.Messages()
	for i, l := 0, msgs.Len(); i < l; i++ {
		targetMsg := msgs.Get(i)
		// check if there is an existing message with the same name
		if existingMsg := existing.Messages().ByName(targetMsg.Name()); existingMsg != nil {
			mergeMessageDescriptors(targetpb.MessageType[existingMsg.Index()], targetMsg, existingMsg)
		}
	}

	// keep custom file options
	if targetpb.Options == nil {
		targetpb.Options = &descriptorpb.FileOptions{}
	}
	mergeUserDefinedOptions(targetpb.Options, target.Options(), existing.Options())

	// keep any custom imports that were added
	targetImports := make(map[string]struct{})
	ti := target.Imports()
	for i, l := 0, ti.Len(); i < l; i++ {
		targetImports[ti.Get(i).Path()] = struct{}{}
	}

	existingImports := existing.Imports()
	for i, l := 0, existingImports.Len(); i < l; i++ {
		existingImport := existingImports.Get(i)

		if _, ok := targetImports[existingImport.Path()]; !ok {
			// only add the import if it's not "github.com/rancher/opni/internal/..."
			if !strings.HasPrefix(existingImport.Path(), "github.com/rancher/opni/internal/") {
				targetpb.Dependency = append(targetpb.Dependency, existingImport.Path())
			}
		}
	}
}

func mergeMessageDescriptors(targetpb *descriptorpb.DescriptorProto, target, existing protoreflect.MessageDescriptor) {
	// tracks numbers that may have collisions, and the field that now has that number
	possibleCollisions := make(map[int32]protoreflect.FieldDescriptor)
	targetFields := target.Fields()
	existingFields := existing.Fields()
	for i, l := 0, targetFields.Len(); i < l; i++ {
		targetField := targetFields.Get(i)
		existingField := existingFields.ByName(targetField.Name())
		if existingField != nil {
			// check if the field number changed
			if existingField.Number() != targetField.Number() {
				// keep the existing field number
				*targetpb.Field[existingField.Index()].Number = int32(existingField.Number())
				possibleCollisions[int32(existingField.Number())] = targetField
			}
			if targetpb.Field[i].Options == nil {
				targetpb.Field[i].Options = &descriptorpb.FieldOptions{}
			}
			mergeUserDefinedOptions(targetpb.Field[i].Options, targetField.Options(), existingField.Options())
		}
	}

	// handle fields that were renumbered. for example:
	/*
		// old
		message Foo {
			string one   = 1;

			string three = 2;
			string four	 = 3;
			string five  = 4;

			string seven = 5;

			string nine  = 6;

		}

		// new (generated)
		message Foo {
			string one   = 1;
			string two   = 2; // <- new field
			string three = 3;
			string four  = 4;
			string five  = 5;
			string six   = 6; // <- new field
			string seven = 7;
			string eight = 8; // <- new field
			string nine  = 9;
			string ten   = 10; // <- new field
		}

		// after initial merge
		message Foo {
			string one   = 1;
			string two   = 2;
			string three = 2; // <- number preserved
			string four  = 3; // <- number preserved
			string five  = 4; // <- number preserved
			string six   = 6;
			string seven = 5; // <- number preserved
			string eight = 8;
			string nine  = 6; // <- number preserved
			string ten   = 10;
		}

		possibleCollisions := {
			2: (&Foo.three),
			3: (&Foo.four),
			4: (&Foo.five),
			5: (&Foo.seven),
			6: (&Foo.nine),
		}

		of these, numbers 2 and 6 were taken, so they need to be renumbered
		using the next available numbers (starting at the largest number + 1)

		// after renumbering
		message Foo {
			string one   = 1;
			string two   = 11; // <- renumbered
			string three = 2;
			string four  = 3;
			string five  = 4;
			string six   = 12; // <- renumbered
			string seven = 5;
			string eight = 8;
			string nine  = 6;
			string ten   = 10;
	*/

	fields := target.Fields()
	var largestNumber protoreflect.FieldNumber
	for i, l := 0, fields.Len(); i < l; i++ {
		largestNumber = max(largestNumber, fields.Get(i).Number())
	}
	nextAvailableNumber := int32(largestNumber + 1)

	for i, l := 0, fields.Len(); i < l; i++ {
		targetField := fields.Get(i)
		if owner, ok := possibleCollisions[int32(targetField.Number())]; ok {
			if targetField == owner {
				continue
			}
			// renumber this field
			*targetpb.Field[targetField.Index()].Number = nextAvailableNumber
			nextAvailableNumber++
		}
	}

	if targetpb.Options == nil {
		targetpb.Options = &descriptorpb.MessageOptions{}
	}
	mergeUserDefinedOptions(targetpb.Options, target.Options(), existing.Options())

	for i, l := 0, target.Messages().Len(); i < l; i++ {
		msg := target.Messages().Get(i)
		if msg.IsMapEntry() {
			continue
		}
		panic("update this code to handle nested messages")
	}
}

func mergeUserDefinedOptions[T protoreflect.ProtoMessage](targetpb proto.Message, targetOpts, existingOpts T) {
	// if there are any user-defined options, preserve them
	proto.RangeExtensions(existingOpts, func(et protoreflect.ExtensionType, v interface{}) bool {
		if et.TypeDescriptor().IsExtension() {
			// flags with the "cli" prefix are generated by the cli plugin and should be overwritten
			if !strings.HasPrefix(string(et.TypeDescriptor().FullName()), "cli.") {
				// any other user-defined option should be preserved exactly
				proto.SetExtension(targetpb, et, v)
			}
		}
		return true
	})
}

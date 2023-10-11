package descriptors

import (
	"flag"
	"fmt"
	"net"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/builder"
	"github.com/rancher/opni/internal/codegen/cli"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
)

type BuilderOptions struct {
	// Tags to use to get field names, in order of preference.
	FieldNameFromTags []string
	// Called for each field. The provided options can be modified before
	// the field is added to the message by returning a mutator function.
	EditFlagOptions func(rf reflect.StructField) func(*cli.FlagOptions)
	// Called for each field. The provided comment can be modified before
	// the field is added to the message.
	EditFieldComment func(rf reflect.StructField, in *string)
	// Called for each field. If it returns true, the field will be skipped.
	SkipFieldFunc func(rf reflect.StructField) bool

	CustomFieldTypes map[reflect.Type]func() *builder.FieldType

	AllScalarFieldsOptional bool
}

type Builder struct {
	BuilderOptions

	cache map[reflect.Type]*builder.MessageBuilder
}

type stdFlagRegistrar = FlagRegistrar[*flag.FlagSet, *flag.Flag]

func BuildMessage[T any](opts ...BuilderOptions) []*builder.MessageBuilder {
	if len(opts) == 0 {
		opts = []BuilderOptions{{}}
	}
	var msg T
	b := Builder{
		BuilderOptions: opts[0],
		cache:          map[reflect.Type]*builder.MessageBuilder{},
	}
	b.BuildMessage(reflect.TypeOf(msg), maybeApplyDiscoveredMetadata(reflect.TypeOf(msg)))

	return fixupMessageNames(reflect.TypeOf(msg), b.cache)
}

// renames any duplicate messages by prefixing with package names until
// all names are unique
func fixupMessageNames(mainMsg reflect.Type, cache map[reflect.Type]*builder.MessageBuilder) []*builder.MessageBuilder {
	renameOneLevelUp := func(t reflect.Type, msg *builder.MessageBuilder) {
		msg.SetName(strcase.ToCamel(path.Base(t.PkgPath())) + msg.GetName())
	}

	seen := map[string]bool{}
	var result []*builder.MessageBuilder

	sortedCache := make([]reflect.Type, 0, len(cache))
	for k := range cache {
		sortedCache = append(sortedCache, k)
	}
	slices.SortFunc(sortedCache, func(a, b reflect.Type) int {
		return strings.Compare(path.Join(a.PkgPath(), a.Name()), path.Join(b.PkgPath(), b.Name()))
	})

	result = append(result, cache[mainMsg])
	seen[mainMsg.Name()] = true

	for _, t := range sortedCache {
		if t == mainMsg {
			continue
		}
		mb := cache[t]
		if t.PkgPath() != mainMsg.PkgPath() {
			renameOneLevelUp(t, mb)
		}
		for seen[mb.GetName()] {
			renameOneLevelUp(t, mb)
		}
		seen[mb.GetName()] = true
		result = append(result, mb)
	}

	return result
}

func maybeApplyDiscoveredMetadata(rt reflect.Type) func(f *builder.FieldBuilder, rf reflect.StructField) {
	if reflect.PointerTo(rt).Implements(reflect.TypeOf((*stdFlagRegistrar)(nil)).Elem()) {
		return autoDiscoverMetadataFromFlags(reflect.New(rt).Interface().(stdFlagRegistrar))
	}
	return nil
}

func (b *Builder) BuildMessage(msgType reflect.Type, newFieldHook func(f *builder.FieldBuilder, rf reflect.StructField)) *builder.MessageBuilder {
	if existing, ok := b.cache[msgType]; ok {
		return existing
	}
	if newFieldHook == nil {
		newFieldHook = func(f *builder.FieldBuilder, rf reflect.StructField) {}
	}
	if b.EditFlagOptions != nil {
		oldFieldHook := newFieldHook
		newFieldHook = func(f *builder.FieldBuilder, rf reflect.StructField) {
			oldFieldHook(f, rf)
			if f.Options == nil {
				f.Options = &descriptorpb.FieldOptions{}
				mutateExtension(f.Options, cli.E_Flag, b.EditFlagOptions(rf))
			}
		}
	}
	if b.EditFieldComment != nil {
		oldFieldHook := newFieldHook
		newFieldHook = func(f *builder.FieldBuilder, rf reflect.StructField) {
			oldFieldHook(f, rf)
			if comments := f.GetComments(); comments != nil {
				b.EditFieldComment(rf, &comments.LeadingComment)
			} else {
				var c string
				b.EditFieldComment(rf, &c)
				if c != "" {
					f.SetComments(builder.Comments{
						LeadingComment: c,
					})
				}
			}
		}
	}
	m := builder.NewMessage(msgType.Name())
FIELDS:
	for i := 0; i < msgType.NumField(); i++ {
		rf := msgType.Field(i)
		if b.SkipFieldFunc != nil {
			if b.SkipFieldFunc(rf) {
				continue
			}
		}
		if !rf.IsExported() {
			continue
		}
		rfName := rf.Name
		rfType := rf.Type
		for _, tagName := range b.FieldNameFromTags {
			tag := rf.Tag.Get(tagName)
			if tag == "-" {
				continue FIELDS
			}
			if name := strings.Split(tag, ",")[0]; name != "" {
				rfName = name
			}
		}
		var isSlice bool
		if rfType.Kind() == reflect.Slice {
			isSlice = true
			rfType = rfType.Elem()
		}
		var isPtr bool
		if rfType.Kind() == reflect.Ptr {
			isPtr = true
			rfType = rfType.Elem()
		}
		// skip fields we can't convert
		switch rfType.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface,
			reflect.UnsafePointer, reflect.Uintptr, reflect.Complex64, reflect.Complex128:
			continue
		}

		// special case: fields that are structs with a single anonymous field and implement fmt.Stringer
		if rfType.Kind() == reflect.Struct && rfType.NumField() == 1 && rfType.Field(0).Anonymous && rfType.Implements(reflect.TypeOf((*fmt.Stringer)(nil)).Elem()) {
			newField := builder.NewField(rfName, builder.FieldTypeString())
			newFieldHook(newField, rf)
			if isSlice {
				newField.SetRepeated()
			}
			m.AddField(newField)
			continue
		}
		// special case: fields that are structs with a single field of type *net.IPNet
		if rfType.Kind() == reflect.Struct && rfType.NumField() == 1 && rfType.Field(0).Type == reflect.TypeOf((*net.IPNet)(nil)) {
			newField := builder.NewField(rfName, builder.FieldTypeString())
			if newField.Options == nil {
				newField.Options = &descriptorpb.FieldOptions{}
			}
			mutateExtension(newField.Options, cli.E_Flag, func(ext *cli.FlagOptions) {
				ext.TypeOverride = "ipNet"
			})
			if isSlice {
				newField.SetRepeated()
			}
			newFieldHook(newField, rf)
			m.AddField(newField)
			continue
		}

		// special case: time.Duration
		if rfType.Name() == "Duration" && rfType.Kind() == reflect.Int64 {
			d, _ := desc.LoadMessageDescriptor("google.protobuf.Duration")
			newField := builder.NewField(rfName, builder.FieldTypeImportedMessage(d))
			if isSlice {
				newField.SetRepeated()
			}
			newFieldHook(newField, rf)
			m.AddField(newField)
			continue
		}

		// add any more special cases here as needed

		if rfType.Kind() == reflect.Map {
			// add map field
			keyType := rfType.Key()
			valueType := rfType.Elem()

			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}

			keyFieldType := b.fieldType(keyType)
			valueFieldType := b.fieldType(valueType)
			newField := builder.NewMapField(rfName, keyFieldType, valueFieldType)
			newFieldHook(newField, rf)
			m.AddField(newField)
			continue
		}

		fieldType := b.fieldType(rfType)
		field := builder.NewField(rfName, fieldType)
		if isSlice {
			field.SetRepeated()
		} else if (b.AllScalarFieldsOptional || isPtr) && isFieldTypeScalar(fieldType.GetType()) {
			// if the field is a pointer to a scalar, set optional
			field.SetProto3Optional(true)
		} else if inlineType, ok := b.canInline(rf, fieldType); ok {
			toInline := b.cache[inlineType.Type]
			field.SetType(builder.FieldTypeMessage(toInline))
			delete(b.cache, rfType)
		}
		newFieldHook(field, rf)
		m.AddField(field)
	}
	b.cache[msgType] = m
	return m
}

func (b *Builder) canInline(rf reflect.StructField, fieldType *builder.FieldType) (reflect.StructField, bool) {
	if fieldType.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return reflect.StructField{}, false
	}
	mb, ok := b.cache[rf.Type]
	if !ok {
		return reflect.StructField{}, false
	}
	children := mb.GetChildren()
	if len(children) != 1 {
		return reflect.StructField{}, false
	}
	if f, ok := children[0].(*builder.FieldBuilder); ok {
		if f.GetType().GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			count := 0
			var inlineType reflect.StructField
			for i := 0; i < rf.Type.NumField(); i++ {
				rfField := rf.Type.Field(i)
				if rfField.Anonymous && rfField.Name == children[0].GetName() {
					inlineType = rfField
					count++
				}
			}
			if count == 1 {
				return inlineType, true
			}
		}
	}
	return reflect.StructField{}, false
}

func (b *Builder) fieldType(rf reflect.Type) *builder.FieldType {
	{
		rf := rf
		if rf.Kind() == reflect.Slice {
			rf = rf.Elem()
		}
		if rf.Kind() == reflect.Pointer {
			rf = rf.Elem()
		}
		if c, ok := b.CustomFieldTypes[rf]; ok {
			return c()
		}
	}
	switch rf.Kind() {
	case reflect.Bool:
		return builder.FieldTypeBool()
	case reflect.Int32, reflect.Int, reflect.Int16, reflect.Int8:
		return builder.FieldTypeInt32()
	case reflect.Int64:
		return builder.FieldTypeInt64()
	case reflect.Uint32, reflect.Uint, reflect.Uint16, reflect.Uint8:
		return builder.FieldTypeUInt32()
	case reflect.Uint64:
		return builder.FieldTypeUInt64()
	case reflect.Float32:
		return builder.FieldTypeFloat()
	case reflect.Float64:
		return builder.FieldTypeDouble()
	case reflect.String:
		return builder.FieldTypeString()
	case reflect.Struct:
		return builder.FieldTypeMessage(b.BuildMessage(rf, maybeApplyDiscoveredMetadata(rf)))
	default:
		panic("unsupported type: " + rf.String() + " (" + rf.Kind().String() + ")")
	}
}

func isScalar(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func isFieldTypeScalar(ft descriptorpb.FieldDescriptorProto_Type) bool {
	switch ft {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL,
		descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
		descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_STRING,
		descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return true
	default:
		return false
	}
}

type generic_flag struct {
	Name     string
	Usage    string
	Value    flag.Value
	DefValue string
}

func newGenericFlag[F any](f F) generic_flag {
	v := reflect.ValueOf(f).Elem()
	return generic_flag{
		Name:     v.FieldByName("Name").String(),
		Usage:    v.FieldByName("Usage").String(),
		Value:    v.FieldByName("Value").Interface().(flag.Value),
		DefValue: v.FieldByName("DefValue").String(),
	}
}

type FlagSet[F any] interface {
	VisitAll(func(F))
}

type FlagRegistrar[FS FlagSet[F], F any] interface {
	RegisterFlags(flags FS)
}

// this code is cursed but works surprisingly well
func autoDiscoverMetadataFromFlags[FR FlagRegistrar[FS, F], FS FlagSet[F], F any](fr FR) func(*builder.FieldBuilder, reflect.StructField) {
	var zero FS
	flagSet := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(FS)
	fr.RegisterFlags(flagSet)
	fieldLookup := map[uintptr]generic_flag{}
	flagSet.VisitAll(func(flag F) {
		g := newGenericFlag(flag)
		v := reflect.ValueOf(g.Value)
		if v.Kind() == reflect.Ptr {
			vptr := v.Pointer()
			if _, ok := fieldLookup[vptr]; !ok {
				fieldLookup[vptr] = g
			}
		}
	})

	return func(f *builder.FieldBuilder, rf reflect.StructField) {
		fieldPointer := reflect.ValueOf(fr).Pointer() + rf.Offset
		if flag, ok := fieldLookup[fieldPointer]; ok {
			// if f is a message type, the pointer could be ambiguous, since the
			// address of the first field is the same as the address of the struct.
			if f.GetType().GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
				flagType := reflect.TypeOf(flag.Value).Elem()
				if flagType.Kind() != rf.Type.Kind() {
					return
				}
			}
			if flag.Usage != "" {
				f.SetComments(builder.Comments{
					LeadingComment: flag.Usage,
				})
			}
			if flag.DefValue != "" {
				if f.Options == nil {
					f.Options = &descriptorpb.FieldOptions{}
					mutateExtension(f.Options, cli.E_Flag, func(ext *cli.FlagOptions) {
						ext.Default = &flag.DefValue
					})
				}
			}
		}
	}
}

func mutateExtension[T any](options proto.Message, ext *protoimpl.ExtensionInfo, mu func(*T)) {
	if mu == nil {
		return
	}
	if !proto.HasExtension(options, ext) {
		t := new(T)
		mu(t)
		proto.SetExtension(options, ext, t)
	} else {
		t := proto.GetExtension(options, ext).(*T)
		mu(t)
		proto.SetExtension(options, ext, t)
	}
}

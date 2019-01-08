package schemabuilder

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
)

// TODO: Enforce keys for items in lists, support compound keys

func makeGraphql(s string) string {
	var b bytes.Buffer
	for i, c := range s {
		if i == 0 {
			b.WriteRune(unicode.ToLower(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func reverseGraphqlFieldName(s string) string {
	var b bytes.Buffer
	for i, c := range s {
		if i == 0 {
			b.WriteRune(unicode.ToUpper(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// graphQLFieldInfo contains basic struct field information related to GraphQL.
type graphQLFieldInfo struct {
	// Skipped indicates that this field should not be included in GraphQL.
	Skipped bool

	// Name is the GraphQL field name that should be exposed for this field.
	Name string

	// KeyField indicates that this field should be treated as a Object Key field.
	KeyField bool
}

// parseGraphQLFieldInfo parses a struct field and returns a struct with the
// parsed information about the field (tag info, name, etc).
func parseGraphQLFieldInfo(field reflect.StructField) (*graphQLFieldInfo, error) {
	if field.PkgPath != "" {
		return &graphQLFieldInfo{Skipped: true}, nil
	}
	tags := strings.Split(field.Tag.Get("graphql"), ",")
	var name string
	if len(tags) > 0 {
		name = tags[0]
	}
	if name == "" {
		name = makeGraphql(field.Name)
	}
	if name == "-" {
		return &graphQLFieldInfo{Skipped: true}, nil
	}

	var key bool

	if len(tags) > 1 {
		for _, tag := range tags[1:] {
			if tag != "key" || key {
				return nil, fmt.Errorf("field %s has unexpected tag %s", name, tag)
			}
			key = true
		}
	}
	return &graphQLFieldInfo{Name: name, KeyField: key}, nil
}

type schemaBuilder struct {
	types        map[reflect.Type]graphql.Type
	objects      map[reflect.Type]*Object
	enumMappings map[reflect.Type]*EnumMapping
	typeCache    map[reflect.Type]cachedType // typeCache maps Go types to GraphQL datatypes
}

type EnumMapping struct {
	Map        map[string]interface{}
	ReverseMap map[interface{}]string
}

// cachedType is a container for GraphQL datatype and the list of its fields
type cachedType struct {
	argType *graphql.InputObject
	fields  map[string]argField
}

var errType reflect.Type
var contextType reflect.Type
var selectionSetType reflect.Type

func init() {
	var err error
	errType = reflect.TypeOf(&err).Elem()
	var context context.Context
	contextType = reflect.TypeOf(&context).Elem()
	var selectionSet *graphql.SelectionSet
	selectionSetType = reflect.TypeOf(selectionSet)
}

func (sb *schemaBuilder) buildField(field reflect.StructField) (*graphql.Field, error) {
	retType, err := sb.getType(field.Type)
	if err != nil {
		return nil, err
	}

	return &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			value := reflect.ValueOf(source)
			if value.Kind() == reflect.Ptr {
				value = value.Elem()
			}
			return value.FieldByIndex(field.Index).Interface(), nil
		},
		Type:           retType,
		ParseArguments: nilParseArguments,
	}, nil
}

func (sb *schemaBuilder) buildUnionStruct(typ reflect.Type) error {
	var name string
	var description string

	if name == "" {
		name = typ.Name()
		if name == "" {
			return fmt.Errorf("bad type %s: should have a name", typ)
		}
	}

	union := &graphql.Union{
		Name:        name,
		Description: description,
		Types:       make(map[string]*graphql.Object),
	}
	sb.types[typ] = union

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" || (field.Anonymous && field.Type == unionType) {
			continue
		}

		if !field.Anonymous {
			return fmt.Errorf("bad type %s: union type member types must be anonymous", name)
		}

		typ, err := sb.getType(field.Type)
		if err != nil {
			return err
		}

		obj, ok := typ.(*graphql.Object)
		if !ok {
			return fmt.Errorf("bad type %s: union type member must be a pointer to a struct, received %s", name, typ.String())
		}

		if union.Types[obj.Name] != nil {
			return fmt.Errorf("bad type %s: union type member may only appear once", name)
		}

		union.Types[obj.Name] = obj
	}
	return nil
}

func (sb *schemaBuilder) buildStruct(typ reflect.Type) error {
	if sb.types[typ] != nil {
		return nil
	}

	if typ == unionType {
		return fmt.Errorf("schemabuilder.Union can only be used as an embedded anonymous non-pointer struct")
	}

	if hasUnionMarkerEmbedded(typ) {
		return sb.buildUnionStruct(typ)
	}

	var name string
	var description string
	var methods Methods
	var objectKey string
	if object, ok := sb.objects[typ]; ok {
		name = object.Name
		description = object.Description
		methods = object.Methods
		objectKey = object.key
	}

	if name == "" {
		name = typ.Name()
		if name == "" {
			return fmt.Errorf("bad type %s: should have a name", typ)
		}
	}

	object := &graphql.Object{
		Name:        name,
		Description: description,
		Fields:      make(map[string]*graphql.Field),
	}
	sb.types[typ] = object

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldInfo, err := parseGraphQLFieldInfo(field)
		if err != nil {
			return fmt.Errorf("bad type %s: %s", typ, fieldInfo.Name)
		}
		if fieldInfo.Skipped {
			continue
		}

		if _, ok := object.Fields[fieldInfo.Name]; ok {
			return fmt.Errorf("bad type %s: two fields named %s", typ, fieldInfo.Name)
		}

		built, err := sb.buildField(field)
		if err != nil {
			return fmt.Errorf("bad field %s on type %s: %s", fieldInfo.Name, typ, err)
		}
		object.Fields[fieldInfo.Name] = built
		if fieldInfo.KeyField {
			if object.Key != nil {
				return fmt.Errorf("bad type %s: multiple key fields", typ)
			}
			if !isScalarType(built.Type) {
				return fmt.Errorf("bad type %s: key type must be scalar, got %T", typ, built.Type)
			}
			object.Key = built.Resolve
		}
	}

	var names []string
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		method := methods[name]

		if method.Paginated {
			typedField, err := sb.buildPaginatedField(typ, method)
			if err != nil {
				return err
			}
			object.Fields[name] = typedField
			continue
		}

		built, err := sb.buildFunction(typ, method)
		if err != nil {
			return fmt.Errorf("bad method %s on type %s: %s", name, typ, err)
		}
		object.Fields[name] = built
	}

	if objectKey != "" {
		keyPtr, ok := object.Fields[objectKey]
		if !ok {
			return fmt.Errorf("key field doesn't exist on object")
		}

		if !isScalarType(keyPtr.Type) {
			return fmt.Errorf("bad type %s: key type must be scalar, got %s", typ, keyPtr.Type.String())
		}
		object.Key = keyPtr.Resolve
	}

	return nil
}

func isScalarType(typ graphql.Type) bool {
	if nonNull, ok := typ.(*graphql.NonNull); ok {
		typ = nonNull.Type
	}
	if _, ok := typ.(*graphql.Scalar); !ok {
		return false
	}
	return true
}

var scalars = map[reflect.Type]string{
	reflect.TypeOf(bool(false)): "bool",
	reflect.TypeOf(int(0)):      "int",
	reflect.TypeOf(int8(0)):     "int8",
	reflect.TypeOf(int16(0)):    "int16",
	reflect.TypeOf(int32(0)):    "int32",
	reflect.TypeOf(int64(0)):    "int64",
	reflect.TypeOf(uint(0)):     "uint",
	reflect.TypeOf(uint8(0)):    "uint8",
	reflect.TypeOf(uint16(0)):   "uint16",
	reflect.TypeOf(uint32(0)):   "uint32",
	reflect.TypeOf(uint64(0)):   "uint64",
	reflect.TypeOf(float32(0)):  "float32",
	reflect.TypeOf(float64(0)):  "float64",
	reflect.TypeOf(string("")):  "string",
	reflect.TypeOf(time.Time{}): "Time",
	reflect.TypeOf([]byte{}):    "bytes",
}

func getScalar(typ reflect.Type) (string, bool) {
	for match, name := range scalars {
		if internal.TypesIdenticalOrScalarAliases(match, typ) {
			return name, true
		}
	}
	return "", false
}

func (sb *schemaBuilder) getEnum(typ reflect.Type) (string, []string, bool) {
	if sb.enumMappings[typ] != nil {
		var values []string
		for mapping := range sb.enumMappings[typ].Map {
			values = append(values, mapping)
		}
		return typ.Name(), values, true
	}
	return "", nil, false
}

func hasUnionMarkerEmbedded(typ reflect.Type) bool {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Anonymous && field.Type == unionType {
			return true
		}
	}
	return false
}

func (sb *schemaBuilder) getType(t reflect.Type) (graphql.Type, error) {
	// Support scalars and optional scalars. Scalars have precedence over structs
	// to have eg. time.Time function as a scalar.
	if typ, values, ok := sb.getEnum(t); ok {
		return &graphql.NonNull{Type: &graphql.Enum{Type: typ, Values: values, ReverseMap: sb.enumMappings[t].ReverseMap}}, nil
	}

	if typ, ok := getScalar(t); ok {
		return &graphql.NonNull{Type: &graphql.Scalar{Type: typ}}, nil
	}
	if t.Kind() == reflect.Ptr {
		if typ, ok := getScalar(t.Elem()); ok {
			return &graphql.Scalar{Type: typ}, nil // XXX: prefix typ with "*"
		}
	}

	// Structs
	if t.Kind() == reflect.Struct {
		if err := sb.buildStruct(t); err != nil {
			return nil, err
		}
		return &graphql.NonNull{Type: sb.types[t]}, nil
	}
	if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
		if err := sb.buildStruct(t.Elem()); err != nil {
			return nil, err
		}
		return sb.types[t.Elem()], nil
	}

	switch t.Kind() {
	case reflect.Slice:
		typ, err := sb.getType(t.Elem())
		if err != nil {
			return nil, err
		}

		// Wrap all slice elements in NonNull.
		if _, ok := typ.(*graphql.NonNull); !ok {
			typ = &graphql.NonNull{Type: typ}
		}

		return &graphql.NonNull{Type: &graphql.List{Type: typ}}, nil

	default:
		return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", t)
	}
}

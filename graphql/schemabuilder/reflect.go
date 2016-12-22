package schemabuilder

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
)

type argParser struct {
	FromJSON func(interface{}, reflect.Value) error
	Type     reflect.Type
}

func nilParseArguments(args interface{}) (interface{}, error) {
	if args == nil {
		return nil, nil
	}
	if args, ok := args.(map[string]interface{}); !ok || len(args) != 0 {
		return nil, graphql.NewSafeError("unexpected args")
	}
	return nil, nil
}

func (p *argParser) Parse(args interface{}) (interface{}, error) {
	if p == nil {
		return nilParseArguments(args)
	}
	parsed := reflect.New(p.Type).Elem()
	if err := p.FromJSON(args, parsed); err != nil {
		return nil, err
	}
	return parsed.Interface(), nil
}

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

var scalarArgParsers = map[reflect.Type]*argParser{
	reflect.TypeOf(bool(false)): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asBool, ok := value.(bool)
			if !ok {
				return errors.New("not a bool")
			}
			dest.Set(reflect.ValueOf(asBool).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf(float64(0)): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asFloat, ok := value.(float64)
			if !ok {
				return errors.New("not a number")
			}
			dest.Set(reflect.ValueOf(asFloat).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf(int64(0)): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asFloat, ok := value.(float64)
			if !ok {
				return errors.New("not a number")
			}
			dest.Set(reflect.ValueOf(int64(asFloat)).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf(int32(0)): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asFloat, ok := value.(float64)
			if !ok {
				return errors.New("not a number")
			}
			dest.Set(reflect.ValueOf(int32(asFloat)).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf(int16(0)): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asFloat, ok := value.(float64)
			if !ok {
				return errors.New("not a number")
			}
			dest.Set(reflect.ValueOf(int16(asFloat)).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf(string("")): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asString, ok := value.(string)
			if !ok {
				return errors.New("not a string")
			}
			dest.Set(reflect.ValueOf(asString).Convert(dest.Type()))
			return nil
		},
	},
	reflect.TypeOf([]byte{}): {
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asString, ok := value.(string)
			if !ok {
				return errors.New("not a string")
			}
			bytes, err := base64.StdEncoding.DecodeString(asString)
			if err != nil {
				return err
			}
			dest.Set(reflect.ValueOf(bytes).Convert(dest.Type()))
			return nil
		},
	},
}

func getScalarArgParser(typ reflect.Type) (*argParser, bool) {
	for match, argParser := range scalarArgParsers {
		if internal.TypesIdenticalOrScalarAliases(match, typ) {
			return argParser, true
		}
	}
	return nil, false
}

func init() {
	for typ, arg := range scalarArgParsers {
		arg.Type = typ
	}
}

type argField struct {
	field    reflect.StructField
	parser   *argParser
	optional bool
}

func makeArgParser(typ reflect.Type) (*argParser, error) {
	if parser, ok := getScalarArgParser(typ); ok {
		return parser, nil
	}

	switch typ.Kind() {
	case reflect.Struct:
		return makeStructParser(typ)
	case reflect.Slice:
		return makeSliceParser(typ)
	case reflect.Ptr:
		return makePtrParser(typ)
	default:
		return nil, fmt.Errorf("bad arg type %s: should be struct, scalar, pointer, or a slice", typ)
	}
}

func makePtrParser(typ reflect.Type) (*argParser, error) {
	inner, err := makeArgParser(typ.Elem())
	if err != nil {
		return nil, err
	}

	return &argParser{
		FromJSON: func(value interface{}, dest reflect.Value) error {
			if value == nil {
				// optional value
				return nil
			}

			ptr := reflect.New(typ.Elem())
			if err := inner.FromJSON(value, ptr.Elem()); err != nil {
				return err
			}
			dest.Set(ptr)
			return nil
		},
		Type: typ,
	}, nil
}

func makeStructParser(typ reflect.Type) (*argParser, error) {
	fields := make(map[string]argField)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}
		if field.Anonymous {
			return nil, fmt.Errorf("bad arg type %s: anonymous fields not supported", typ)
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
			continue
		}

		var key bool

		if len(tags) > 1 {
			for _, tag := range tags[1:] {
				if tag != "key" || key {
					return nil, fmt.Errorf("bad type %s: field %s has unexpected tag %s", typ, name, tag)
				}
				key = true
			}
		}

		if _, ok := fields[name]; ok {
			return nil, fmt.Errorf("bad arg type %s: duplicate field %s", typ, name)
		}

		parser, err := makeArgParser(field.Type)
		if err != nil {
			return nil, err
		}

		fields[name] = argField{
			field:  field,
			parser: parser,
		}
	}

	return &argParser{
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asMap, ok := value.(map[string]interface{})
			if !ok {
				return errors.New("not an object")
			}

			for name, field := range fields {
				value := asMap[name]
				fieldDest := dest.FieldByIndex(field.field.Index)
				if err := field.parser.FromJSON(value, fieldDest); err != nil {
					return fmt.Errorf("%s: %s", name, err)
				}
			}

			for name := range asMap {
				if _, ok := fields[name]; !ok {
					return fmt.Errorf("unknown arg %s", name)
				}
			}

			return nil
		},
		Type: typ,
	}, nil
}

func makeSliceParser(typ reflect.Type) (*argParser, error) {
	inner, err := makeArgParser(typ.Elem())
	if err != nil {
		return nil, err
	}

	return &argParser{
		FromJSON: func(value interface{}, dest reflect.Value) error {
			asSlice, ok := value.([]interface{})
			if !ok {
				return errors.New("not a list")
			}

			dest.Set(reflect.MakeSlice(typ, len(asSlice), len(asSlice)))

			for i, value := range asSlice {
				if err := inner.FromJSON(value, dest.Index(i)); err != nil {
					return err
				}
			}

			return nil
		},
		Type: typ,
	}, nil
}

type schemaBuilder struct {
	types   map[reflect.Type]graphql.Type
	objects map[reflect.Type]Object
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

func (sb *schemaBuilder) buildFunction(typ reflect.Type, fun reflect.Value) (*graphql.Field, error) {
	ptr := reflect.PtrTo(typ)

	if fun.Kind() != reflect.Func {
		return nil, fmt.Errorf("fun must be func, not %s", fun)
	}
	funcType := fun.Type()

	in := make([]reflect.Type, 0, funcType.NumIn())
	for i := 0; i < funcType.NumIn(); i++ {
		in = append(in, funcType.In(i))
	}

	var argParser *argParser
	var ptrFunc bool
	var hasContext, hasSource, hasArgs, hasSelectionSet bool

	if len(in) > 0 && in[0] == contextType {
		hasContext = true
		in = in[1:]
	}

	if len(in) > 0 && (in[0] == typ || in[0] == ptr) {
		hasSource = true
		ptrFunc = in[0] == ptr
		in = in[1:]
	}

	if len(in) > 0 && in[0] != selectionSetType {
		hasArgs = true
		var err error
		if argParser, err = makeArgParser(in[0]); err != nil {
			return nil, err
		}
		in = in[1:]
	}

	if len(in) > 0 && in[0] == selectionSetType {
		hasSelectionSet = true
		in = in[:len(in)-1]
	}

	// We have succeeded if no arguments remain.
	if len(in) != 0 {
		return nil, fmt.Errorf("%s arguments should be [context][, [*]%s][, args][, selectionSet]", funcType, typ)
	}

	// Parse return values. The first return value must be the actual value, and
	// the second value can optionally be an error.

	out := make([]reflect.Type, 0, funcType.NumOut())
	for i := 0; i < funcType.NumOut(); i++ {
		out = append(out, funcType.Out(i))
	}

	var hasRet, hasError bool

	if len(out) > 0 && out[0] != errType {
		hasRet = true
		out = out[1:]
	}

	if len(out) > 0 && out[0] == errType {
		hasError = true
		out = out[1:]
	}

	if len(out) != 0 {
		return nil, fmt.Errorf("%s return values should [result][, error]", funcType)
	}

	var retType graphql.Type
	if hasRet {
		var err error
		retType, err = sb.getType(funcType.Out(0))
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		retType, err = sb.getType(reflect.TypeOf(true))
		if err != nil {
			return nil, err
		}
	}

	return &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			// Set up function arguments.
			in := make([]reflect.Value, 0, funcType.NumIn())

			if hasContext {
				in = append(in, reflect.ValueOf(ctx))
			}

			// Set up source.
			if hasSource {
				sourceValue := reflect.ValueOf(source)
				ptrSource := sourceValue.Kind() == reflect.Ptr
				switch {
				case ptrSource && !ptrFunc:
					in = append(in, sourceValue.Elem())
				case !ptrSource && ptrFunc:
					copyPtr := reflect.New(typ)
					copyPtr.Elem().Set(sourceValue)
					in = append(in, copyPtr)
				default:
					in = append(in, sourceValue)
				}
			}

			// Set up other arguments.
			if hasArgs {
				in = append(in, reflect.ValueOf(args))
			}
			if hasSelectionSet {
				in = append(in, reflect.ValueOf(selectionSet))
			}

			// Call the function.
			out := fun.Call(in)

			var result interface{}
			if hasRet {
				result = out[0].Interface()
				out = out[1:]
			} else {
				result = true
			}
			if hasError {
				if err := out[0]; !err.IsNil() {
					return nil, err.Interface().(error)
				}
			}

			return result, nil
		},
		Type:           retType,
		ParseArguments: argParser.Parse,
		Expensive:      hasContext,
	}, nil
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

func (sb *schemaBuilder) buildStruct(typ reflect.Type) error {
	if sb.types[typ] != nil {
		return nil
	}

	var name string
	var description string
	var methods Methods
	if object, ok := sb.objects[typ]; ok {
		name = object.Name
		description = object.Description
		methods = object.Methods
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
		if field.PkgPath != "" {
			continue
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
			continue
		}

		var key bool

		if len(tags) > 1 {
			for _, tag := range tags[1:] {
				if tag != "key" || key {
					return fmt.Errorf("bad type %s: field %s has unexpected tag %s", typ, name, tag)
				}
				key = true
			}
		}

		if _, ok := object.Fields[name]; ok {
			return fmt.Errorf("bad type %s: two fields named %s", typ, name)
		}

		built, err := sb.buildField(field)
		if err != nil {
			return fmt.Errorf("bad field %s on type %s: %s", name, typ, err)
		}
		object.Fields[name] = built
		if key {
			if object.Key != nil {
				return fmt.Errorf("bad type %s: multiple key fields", typ)
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

		built, err := sb.buildFunction(typ, reflect.ValueOf(method))
		if err != nil {
			return fmt.Errorf("bad method %s on type %s: %s", name, typ, err)
		}
		object.Fields[name] = built
	}

	return nil
}

var scalars = map[reflect.Type]bool{
	reflect.TypeOf(bool(false)): true,
	reflect.TypeOf(int(0)):      true,
	reflect.TypeOf(int8(0)):     true,
	reflect.TypeOf(int16(0)):    true,
	reflect.TypeOf(int32(0)):    true,
	reflect.TypeOf(int64(0)):    true,
	reflect.TypeOf(uint(0)):     true,
	reflect.TypeOf(uint8(0)):    true,
	reflect.TypeOf(uint16(0)):   true,
	reflect.TypeOf(uint32(0)):   true,
	reflect.TypeOf(uint64(0)):   true,
	reflect.TypeOf(float32(0)):  true,
	reflect.TypeOf(float64(0)):  true,
	reflect.TypeOf(string("")):  true,
	reflect.TypeOf(time.Time{}): true,
	reflect.TypeOf([]byte{}):    true,
}

func getScalar(typ reflect.Type) (string, bool) {
	for match := range scalars {
		if internal.TypesIdenticalOrScalarAliases(match, typ) {
			return typ.String(), true
		}
	}
	return "", false
}

func (sb *schemaBuilder) getType(t reflect.Type) (graphql.Type, error) {
	if sb.types[t] != nil {
		return sb.types[t], nil
	}

	// Support scalars and optional scalars. Scalars have precedence over structs
	// to have eg. time.Time function as a scalar.
	if typ, ok := getScalar(t); ok {
		return &graphql.Scalar{Type: typ}, nil
	}
	if t.Kind() == reflect.Ptr {
		if typ, ok := getScalar(t.Elem()); ok {
			return &graphql.Scalar{Type: "*" + typ}, nil
		}
	}

	// Structs
	if t.Kind() == reflect.Struct {
		if err := sb.buildStruct(t); err != nil {
			return nil, err
		}
		return sb.types[t], nil
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
		return &graphql.List{Type: typ}, nil

	default:
		return nil, fmt.Errorf("bad type %s: should be a scalar, slice, or struct type", t)
	}
}

type query struct{}
type mutation struct{}

type Server interface {
	Query() Object
	Mutation() Object
}

func BuildSchema(server Server) (*graphql.Schema, error) {
	// build objects by calling methods on server
	var objects []Object

	value := reflect.ValueOf(server)
	serverTyp := value.Type()

	var queryObject, mutationObject Object

	for i := 0; i < serverTyp.NumMethod(); i++ {
		method := serverTyp.Method(i)
		if method.Type.NumIn() == 1 && method.Type.NumOut() == 1 && method.Type.Out(0) == reflect.TypeOf(Object{}) {
			object := method.Func.Call([]reflect.Value{value})[0].Interface().(Object)

			if method.Name == "Query" {
				if object.Type == nil {
					object.Type = query{}
				}
				if object.Name == "" {
					object.Name = "Query"
				}
				queryObject = object
			}

			if method.Name == "Mutation" {
				if object.Type == nil {
					object.Type = mutation{}
				}
				if object.Name == "" {
					object.Name = "Mutation"
				}
				mutationObject = object
			}

			objects = append(objects, object)
		}
	}

	sb := &schemaBuilder{
		types:   make(map[reflect.Type]graphql.Type),
		objects: make(map[reflect.Type]Object),
	}

	for _, object := range objects {
		typ := reflect.TypeOf(object.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("object.Type should be a struct, not %s", typ.String())
		}

		if _, ok := sb.objects[typ]; ok {
			return nil, fmt.Errorf("duplicate object for %s", typ.String())
		}

		sb.objects[typ] = object
	}

	queryTyp, err := sb.getType(reflect.TypeOf(queryObject.Type))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(mutationObject.Type))
	if err != nil {
		return nil, err
	}
	return &graphql.Schema{
		Query:    queryTyp,
		Mutation: mutationTyp,
	}, nil
}

// MustBuildSchema builds a schema and panics if an error occurs
func MustBuildSchema(server Server) *graphql.Schema {
	built, err := BuildSchema(server)
	if err != nil {
		panic(err)
	}
	return built
}

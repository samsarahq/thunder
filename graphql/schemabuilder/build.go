package schemabuilder

import (
	"fmt"
	"reflect"
	"time"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/internal"
)

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

func getScalar(typ reflect.Type) (string, bool) {
	for match, name := range scalars {
		if internal.TypesIdenticalOrScalarAliases(match, typ) {
			return name, true
		}
	}
	return "", false
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

package schemabuilder

import (
	"fmt"
	"reflect"

	"github.com/samsarahq/thunder/graphql"
)

type Schema struct {
	objects   map[string]*Object
	enumTypes map[reflect.Type]*EnumMapping
}

func NewSchema() *Schema {
	schema := &Schema{
		objects: make(map[string]*Object),
	}

	// Default registrations.
	schema.Enum(SortOrder(0), map[string]SortOrder{
		"asc":  SortOrder_Ascending,
		"desc": SortOrder_Descending,
	})

	return schema
}

// Enum registers an enumType in the schema. The val should be any arbitrary value
// of the enumType to be used for reflection, and the enumMap should be
// the corresponding map of the enums.
//
// For example a enum could be declared as follows:
//   type enumType int32
//   const (
//	  one   enumType = 1
//	  two   enumType = 2
//	  three enumType = 3
//   )
//
// Then the Enum can be registered as:
//   s.Enum(enumType(1), map[string]interface{}{
//     "one":   enumType(1),
//     "two":   enumType(2),
//     "three": enumType(3),
//   })
func (s *Schema) Enum(val interface{}, enumMap interface{}) {
	typ := reflect.TypeOf(val)
	if s.enumTypes == nil {
		s.enumTypes = make(map[reflect.Type]*EnumMapping)
	}

	eMap, rMap := getEnumMap(enumMap, typ)
	s.enumTypes[typ] = &EnumMapping{Map: eMap, ReverseMap: rMap}
}

func getEnumMap(enumMap interface{}, typ reflect.Type) (map[string]interface{}, map[interface{}]string) {
	rMap := make(map[interface{}]string)
	eMap := make(map[string]interface{})
	v := reflect.ValueOf(enumMap)
	if v.Kind() == reflect.Map {
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)
			valInterface := val.Interface()
			if reflect.TypeOf(valInterface).Kind() != typ.Kind() {
				panic("types are not equal")
			}
			if key.Kind() == reflect.String {
				mapVal := reflect.ValueOf(valInterface).Convert(typ)
				eMap[key.String()] = mapVal.Interface()
			} else {
				panic("keys are not strings")
			}
		}
	} else {
		panic("enum function not passed a map")
	}

	for key, val := range eMap {
		rMap[val] = key
	}
	return eMap, rMap

}

func (s *Schema) Object(name string, typ interface{}) *Object {
	if object, ok := s.objects[name]; ok {
		if reflect.TypeOf(object.Type) != reflect.TypeOf(typ) {
			panic("re-registered object with different type")
		}
		return object
	}
	object := &Object{
		Name: name,
		Type: typ,
	}
	s.objects[name] = object
	return object
}

type query struct{}

func (s *Schema) Query() *Object {
	return s.Object("Query", query{})
}

type mutation struct{}

func (s *Schema) Mutation() *Object {
	return s.Object("Mutation", mutation{})
}

func (s *Schema) Build() (*graphql.Schema, error) {
	sb := &schemaBuilder{
		types:        make(map[reflect.Type]graphql.Type),
		objects:      make(map[reflect.Type]*Object),
		enumMappings: s.enumTypes,
		typeCache:    make(map[reflect.Type]cachedType, 0),
	}

	for _, object := range s.objects {
		typ := reflect.TypeOf(object.Type)
		if typ.Kind() != reflect.Struct {
			return nil, fmt.Errorf("object.Type should be a struct, not %s", typ.String())
		}

		if _, ok := sb.objects[typ]; ok {
			return nil, fmt.Errorf("duplicate object for %s", typ.String())
		}

		sb.objects[typ] = object
	}

	queryTyp, err := sb.getType(reflect.TypeOf(&query{}))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(&mutation{}))
	if err != nil {
		return nil, err
	}
	return &graphql.Schema{
		Query:    queryTyp,
		Mutation: mutationTyp,
	}, nil
}

// MustBuildSchema builds a schema and panics if an error occurs
func (s *Schema) MustBuild() *graphql.Schema {
	built, err := s.Build()
	if err != nil {
		panic(err)
	}
	return built
}

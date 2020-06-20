package schemabuilder

import (
	"fmt"
	"reflect"

	"github.com/samsarahq/thunder/graphql"
)

const federationField = "__federation"

// Schema is a struct that can be used to build out a GraphQL schema.  Functions
// can be registered against the "Mutation" and "Query" objects in order to
// build out a full GraphQL schema.
type Schema struct {
	Name      string
	objects   map[string]*Object
	enumTypes map[reflect.Type]*EnumMapping
}

// NewSchema creates a new schema.
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

// NewSchema creates a new schema with a schema name
func NewSchemaWithName(name string) *Schema {
	schema := &Schema{
		Name:    name,
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

// OpjectOption is an interface for the variadic options that can be passed
// to a Object for configuring options on that object.
type ObjectOption interface {
	apply(*Object)
}

// objectOptionFunc is a helper to define ObjectOptions when creating an object
type objectOptionFunc func(*Object)

func (f objectOptionFunc) apply(m *Object) { f(m) }

// RootObject is an option that can be passed to a Object to indicate that the object
// can have field funcs on other severs, allowing it to be federated.
var RootObject objectOptionFunc = func(m *Object) {
	m.IsRoot = true
}

// TODO(zhekai): comment
var ShadowObject objectOptionFunc = func(m *Object) {
	m.IsShadowObject = true
}

// Object registers a struct as a GraphQL Object in our Schema.
// (https://facebook.github.io/graphql/June2018/#sec-Objects)
// We'll read the fields of the struct to determine it's basic "Fields" and
// we'll return an Object struct that we can use to register custom
// relationships and fields on the object.
func (s *Schema) Object(name string, typ interface{}, options ...ObjectOption) *Object {
	if object, ok := s.objects[name]; ok {
		if reflect.TypeOf(object.Type) != reflect.TypeOf(typ) {
			panic("re-registered object with different type")
		}
		return object
	}
	object := &Object{
		Name:        name,
		Type:        typ,
		ServiceName: s.Name,
	}
	s.objects[name] = object

	for _, opt := range options {
		opt.apply(object)
	}

	if object.IsRoot {
		federatedObjectType := reflect.New(reflect.TypeOf(typ)).Interface()
		if object.Methods == nil {
			object.Methods = make(Methods)
		}

		federatedMethod := &method{}
		if _, ok := object.Methods[federationField]; ok {
			panic("duplicate federation method")
		}
		federatedMethod.FederationType = federatedObjectType
		object.Methods[federationField] = federatedMethod
	}

	return object
}

type query struct{}

// Query returns an Object struct that we can use to register all the top level
// graphql query functions we'd like to expose.
func (s *Schema) Query() *Object {
	return s.Object("Query", query{}, RootObject)
}

type mutation struct{}

// Mutation returns an Object struct that we can use to register all the top level
// graphql mutations functions we'd like to expose.
func (s *Schema) Mutation() *Object {
	return s.Object("Mutation", mutation{})
}

// Build takes the schema we have built on our Query and Mutation starting
// points and builds a full graphql.Schema we can use to execute and run
// queries.  Essentially we read through all the methods we've attached to our
// Query and Mutation Objects and ensure that those functions are returning
// other Objects that we can resolve in our GraphQL graph.
func (s *Schema) Build() (*graphql.Schema, error) {
	sb := &schemaBuilder{
		types:        make(map[reflect.Type]graphql.Type),
		typeNames:    make(map[string]reflect.Type),
		objects:      make(map[reflect.Type]*Object),
		enumMappings: s.enumTypes,
		typeCache:    make(map[reflect.Type]cachedType, 0),
	}

	queryObject := s.Object("Query", query{}, RootObject)
	//s.Object("Query", query{})
	s.Object("Mutation", mutation{})

	var federationObject *Object
	if queryObject.IsRoot {
		federationObject = s.Object("Federation", federation{})
		if _, ok := queryObject.Methods["__federation"]; !ok {
			queryObject.FieldFunc("__federation", func() federation { return federation{} })
		}
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

		// add Federation shadow object field func to Query

		if object.IsShadowObject {
			if federationObject == nil {
				return nil, fmt.Errorf("root query should be federated")
			}
			if federationObject.Methods == nil {
				federationObject.Methods = make(Methods)
			}
			m := &method{}
			m.ShadowObjectType = object.Type
			federationMethodName := fmt.Sprintf("%s-%s", object.Name, object.ServiceName)
			if _, ok := federationObject.Methods[federationMethodName]; ok {
				panic("duplicate federation method")
			}
			federationObject.Methods[federationMethodName] = m
		}

	}

	queryTyp, err := sb.getType(reflect.TypeOf(&query{}))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(&mutation{}))
	if err != nil {
		return nil, err
	}
	//pretty.Println("zhekai-query", queryTyp)
	return &graphql.Schema{
		Query:    queryTyp,
		Mutation: mutationTyp,
	}, nil
}

// MustBuildSchema builds a schema and panics if an error occurs.
func (s *Schema) MustBuild() *graphql.Schema {
	built, err := s.Build()
	if err != nil {
		panic(err)
	}
	return built
}

type federation struct{}

// Federation returns an object struct for exposing federated objects.
func (s *Schema) Federation() *Object {
	q := s.Query()
	if _, ok := q.Methods["__federation"]; !ok {
		q.FieldFunc("__federation", func() federation { return federation{} })
	}
	return s.Object("Federation", federation{})
}

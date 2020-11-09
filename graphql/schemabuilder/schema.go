package schemabuilder

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/northvolt/thunder/graphql"
)

const federationField = "_federation"
const federationName = "Federation"

// Schema is a struct that can be used to build out a GraphQL schema.  Functions
// can be registered against the "Mutation" and "Query" objects in order to
// build out a full GraphQL schema.
type Schema struct {
	Name      string
	objects   map[string]*Object
	ifaces    map[string]*Object
	enumTypes map[reflect.Type]*EnumMapping

	scalars       map[reflect.Type]string
	ifaceStrategy IfaceStrategy
}

// SchemaOption specifies functionality for the schema.
type SchemaOption func(*Schema)

// WithIfaceStrategy specifies the strategy that should be used
// to translate interfaces to go types.
func WithIfaceStrategy(is IfaceStrategy) SchemaOption {
	return func(s *Schema) {
		s.ifaceStrategy = is
	}
}

// WithScalars specifies a set of scalars.
func WithScalars(scalars map[reflect.Type]string) SchemaOption {
	return func(s *Schema) {
		s.scalars = scalars
	}
}

// NewSchema creates a new schema.
func NewSchema(opts ...SchemaOption) *Schema {
	schema := &Schema{
		objects:       make(map[string]*Object),
		ifaces:        make(map[string]*Object),
		ifaceStrategy: IfaceGetterStrategy,
	}

	for _, o := range opts {
		o(schema)
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
		Name:    strings.ToLower(name),
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
	apply(*Schema, *Object)
}

// objectOptionFunc is a helper to define ObjectOptions when creating an object
type objectOptionFunc func(*Schema, *Object)

func (f objectOptionFunc) apply(s *Schema, m *Object) { f(s, m) }

type federation struct{}

func FetchObjectFromKeys(f interface{}, options ...ObjectOption) ObjectOption {
	// Create a method on the "Federation" object to create the shadow object from the federated keys
	m := &method{Fn: f, Expensive: true}

	var FetchObjectFromKeysField objectOptionFunc = func(s *Schema, obj *Object) {
		q := s.Query()
		if _, ok := q.Methods[federationField]; !ok {
			q.FieldFunc(federationField, func() federation { return federation{} })
		}
		fedObj := s.Object(federationName, federation{})

		if fedObj.Methods == nil {
			fedObj.Methods = make(Methods)
		}

		federatedMethodName := fmt.Sprintf("%s_%s", obj.ServiceName, obj.Name)
		if _, ok := fedObj.Methods[federatedMethodName]; ok {
			panic("duplicate method")
		}

		fedObj.Methods[federatedMethodName] = m

		if obj.Methods == nil {
			obj.Methods = make(Methods)
		}
		objectType := reflect.PtrTo(reflect.TypeOf(obj.Type))
		rootMethod := &method{
			RootObjectType: objectType,
		}
		if _, ok := obj.Methods[federationField]; ok {
			panic("duplicate federation method")
		}
		obj.Methods[federationField] = rootMethod
	}
	return FetchObjectFromKeysField
}

func (s *Schema) Interface(name string, typ interface{}, options ...ObjectOption) *Object {
	if iface, ok := s.ifaces[name]; ok {
		if reflect.TypeOf(iface.Type) != reflect.TypeOf(typ) {
			panic("re-registered object with different type")
		}
		return iface
	}

	iface := &Object{
		Name:        name,
		Type:        typ,
		ServiceName: s.Name,
		IsInterface: true,
	}
	s.ifaces[name] = iface

	for _, opt := range options {
		opt.apply(s, iface)
	}

	return iface
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
		opt.apply(s, object)
	}

	s.updateLinks()

	return object
}

type query struct{}

// Query returns an Object struct that we can use to register all the top level
// graphql query functions we'd like to expose.
func (s *Schema) Query() *Object {
	return s.Object("Query", query{})
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
		types:         make(map[reflect.Type]graphql.Type),
		typeNames:     make(map[string]reflect.Type),
		objects:       make(map[reflect.Type]*Object),
		ifaces:        make(map[reflect.Type]*Object),
		enumMappings:  s.enumTypes,
		typeCache:     make(map[reflect.Type]cachedType, 0),
		ifaceStrategy: s.ifaceStrategy,
		scalars:       mergeScalars(defaultScalars(), s.scalars),
	}

	s.Object("Query", query{})
	s.Object("Mutation", mutation{})

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

	for _, iface := range s.ifaces {
		typ := reflect.TypeOf(iface.Type).Elem()
		if _, ok := sb.ifaces[typ]; ok {
			return nil, fmt.Errorf("duplicate object for %s", typ.String())
		}
		sb.ifaces[typ] = iface
	}

	queryTyp, err := sb.getType(reflect.TypeOf(&query{}))
	if err != nil {
		return nil, err
	}
	mutationTyp, err := sb.getType(reflect.TypeOf(&mutation{}))
	if err != nil {
		return nil, err
	}

	var objects []graphql.Type
	for t := range sb.objects {
		tt, err := sb.getType(t)
		if err != nil {
			return nil, fmt.Errorf("objects: get type: %s: %w", t, err)
		}
		objects = append(objects, tt)
	}

	var ifaces []graphql.Type
	for t := range sb.ifaces {
		tt, err := sb.getType(t)
		if err != nil {
			return nil, fmt.Errorf("ifaces: get type: %s: %w", t, err)
		}
		ifaces = append(ifaces, tt)
	}

	if err := sb.validateInterfaces(); err != nil {
		return nil, err
	}

	return &graphql.Schema{
		Query:    queryTyp,
		Mutation: mutationTyp,
		Objects:  objects,
		Ifaces:   ifaces,
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

func (s *Schema) updateLinks() {
	for _, obj := range s.objects {
		obj.Interfaces = s.findInterfaces(obj.Type)
	}
	for _, iface := range s.ifaces {
		iface.PossibleTypes = s.findPossibleTypes(iface.Type)
	}
}

func (s *Schema) findPossibleTypes(v interface{}) []reflect.Type {
	iface := reflect.TypeOf(v).Elem()
	var out []reflect.Type
	for _, obj := range s.objects {
		if obj.IsInterface {
			continue
		}
		t := reflect.TypeOf(obj.Type)
		if reflect.PtrTo(t).Implements(iface) {
			out = append(out, t)
		}
	}
	return out
}

func (s *Schema) findInterfaces(v interface{}) []reflect.Type {
	impl := reflect.PtrTo(reflect.TypeOf(v))
	var out []reflect.Type
	for _, obj := range s.ifaces {
		if !obj.IsInterface {
			continue
		}
		t := reflect.TypeOf(obj.Type).Elem()
		if impl.Implements(t) {
			out = append(out, t)
		}
	}
	return out
}

func mergeScalars(a, b map[reflect.Type]string) map[reflect.Type]string {
	out := make(map[reflect.Type]string)
	if a != nil {
		for k, v := range a {
			out[k] = v
		}
	}
	if b != nil {
		for k, v := range b {
			out[k] = v
		}
	}
	return out
}

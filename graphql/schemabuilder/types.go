package schemabuilder

import (
	"context"
	"reflect"
)

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name        string // Optional, defaults to Type's name.
	Description string
	Type        interface{}
	Methods     Methods // Deprecated, use FieldFunc instead.

	key string
}

type paginationObject struct {
	Name string
	Fn   interface{}
}

// FieldFuncOption is an interface for the variadic options that can be passed
// to a FieldFunc for configuring options on that function.
type FieldFuncOption interface {
	apply(*method)
}

// fieldFuncOptionFunc is a helper to define FieldFuncOptions from a func.
type fieldFuncOptionFunc func(*method)

func (f fieldFuncOptionFunc) apply(m *method) { f(m) }

// NonNullable is an option that can be passed to a FieldFunc to indicate that
// its return value is required, even if the return value is a pointer type.
var NonNullable fieldFuncOptionFunc = func(m *method) {
	m.MarkedNonNullable = true
}

// Paginated is an option that can be passed to a FieldFunc to indicate that
// its return value should be paginated.
var Paginated fieldFuncOptionFunc = func(m *method) {
	m.Paginated = true
}

type Filter struct {
	Name            string
	FilterFunc      interface{}
	BatchFilterFunc interface{}
	FallbackFlag    func(context.Context) bool
	Options         []FieldFuncOption
}

func FilterField(name string, filter interface{}, options ...FieldFuncOption) FieldFuncOption {
	var filterStruct = Filter{
		Name:       name,
		FilterFunc: filter,
		Options:    options,
	}
	var fieldFuncTextFilterFields fieldFuncOptionFunc = func(m *method) {
		if m.TextFilterFuncs == nil {
			m.TextFilterFuncs = map[string]Filter{}
		}
		if filter, ok := m.TextFilterFuncs[name]; ok {
			panic("Batch Field Filters have the same name: " + filter.Name)
		}
		m.TextFilterFuncs[name] = filterStruct
	}
	return fieldFuncTextFilterFields
}

func BatchFilterField(name string, batchFilter interface{}, options ...FieldFuncOption) FieldFuncOption {
	var filterStruct = Filter{
		Name:            name,
		BatchFilterFunc: batchFilter,
		Options:         options,
	}
	var fieldFuncTextFilterFields fieldFuncOptionFunc = func(m *method) {
		if m.TextFilterFuncs == nil {
			m.TextFilterFuncs = map[string]Filter{}
		}
		if filter, ok := m.TextFilterFuncs[name]; ok {
			panic("Batch Field Filters have the same name: " + filter.Name)
		}
		m.TextFilterFuncs[name] = filterStruct
	}
	return fieldFuncTextFilterFields
}

func BatchFilterFieldWithFallback(name string, batchFilter interface{}, filter interface{}, flag func(context.Context) bool, options ...FieldFuncOption) FieldFuncOption {
	var filterStruct = Filter{
		Name:            name,
		FilterFunc:      filter,
		BatchFilterFunc: batchFilter,
		FallbackFlag:    flag,
		Options:         options,
	}
	var fieldFuncTextFilterFields fieldFuncOptionFunc = func(m *method) {
		if m.TextFilterFuncs == nil {
			m.TextFilterFuncs = map[string]Filter{}
		}
		if filter, ok := m.TextFilterFuncs[name]; ok {
			panic("Batch Field Filters have the same name: " + filter.Name)
		}
		m.TextFilterFuncs[name] = filterStruct
	}
	return fieldFuncTextFilterFields
}

type SortFields map[string]interface{}

func (s SortFields) apply(m *method) {
	m.SortFuncs = s
}

// FieldFunc exposes a field on an object. The function f can take a number of
// optional arguments:
// func([ctx context.Context], [o *Type], [args struct {}]) ([Result], [error])
//
// For example, for an object of type User, a fullName field might take just an
// instance of the object:
//    user.FieldFunc("fullName", func(u *User) string {
//       return u.FirstName + " " + u.LastName
//    })
//
// An addUser mutation field might take both a context and arguments:
//    mutation.FieldFunc("addUser", func(ctx context.Context, args struct{
//        FirstName string
//        LastName  string
//    }) (int, error) {
//        userID, err := db.AddUser(ctx, args.FirstName, args.LastName)
//        return userID, err
//    })
func (s *Object) FieldFunc(name string, f interface{}, options ...FieldFuncOption) {
	if s.Methods == nil {
		s.Methods = make(Methods)
	}

	m := &method{Fn: f}
	for _, opt := range options {
		opt.apply(m)
	}

	if _, ok := s.Methods[name]; ok {
		panic("duplicate method")
	}
	s.Methods[name] = m
}

func (s *Object) BatchFieldFunc(name string, batchFunc interface{}, options ...FieldFuncOption) {
	if s.Methods == nil {
		s.Methods = make(Methods)
	}

	m := &method{
		Fn:    batchFunc,
		Batch: true,
	}
	for _, opt := range options {
		opt.apply(m)
	}

	if _, ok := s.Methods[name]; ok {
		panic("duplicate method")
	}
	s.Methods[name] = m
}

func (s *Object) BatchFieldFuncWithFallback(name string, batchFunc interface{}, fallbackFunc interface{}, flag UseFallbackFlag, options ...FieldFuncOption) {
	if s.Methods == nil {
		s.Methods = make(Methods)
	}

	m := &method{
		Fn: batchFunc,
		BatchArgs: batchArgs{
			FallbackFunc:          fallbackFunc,
			ShouldUseFallbackFunc: flag,
		},
		Batch: true,
	}
	for _, opt := range options {
		opt.apply(m)
	}

	if _, ok := s.Methods[name]; ok {
		panic("duplicate method")
	}
	s.Methods[name] = m
}

// Key registers the key field on an object. The field should be specified by the name of the
// graphql field.
// For example, for an object User:
//   type struct User {
//	   UserKey int64
//   }
// The key will be registered as:
// object.Key("userKey")
func (s *Object) Key(f string) {
	s.key = f
}

type method struct {
	MarkedNonNullable bool
	Fn                interface{}

	// Whether or not the FieldFunc is paginated.
	Paginated bool

	// Text filter methods
	TextFilterFuncs map[string]Filter

	// Sort methods
	SortFuncs map[string]interface{}

	// Whether the FieldFunc is a batchField
	Batch bool

	BatchArgs batchArgs
}

type UseFallbackFlag func(context.Context) bool

type batchArgs struct {
	FallbackFunc          interface{}
	ShouldUseFallbackFunc UseFallbackFlag
}

// A Methods map represents the set of methods exposed on a Object.
type Methods map[string]*method

// Union is a special marker struct that can be embedded into to denote
// that a type should be treated as a union type by the schemabuilder.
//
// For example, to denote that a return value that may be a *Asset or
// *Vehicle might look like:
//   type GatewayUnion struct {
//     schemabuilder.Union
//     *Asset
//     *Vehicle
//   }
//
// Fields returning a union type should expect to return this type as a
// one-hot struct, i.e. only Asset or Vehicle should be specified, but not both.
type Union struct{}

var unionType = reflect.TypeOf(Union{})

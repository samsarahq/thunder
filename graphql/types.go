package graphql

import (
	"context"
	"fmt"
)

// Type represents a GraphQL type, and should be either an Object, a Scalar,
// or a List
type Type interface {
	String() string

	// isType() is a no-op used to tag the known values of Type, to prevent
	// arbitrary interface{} from implementing Type
	isType()
}

// Scalar is a leaf value
type Scalar struct {
	Type string
}

func (s *Scalar) isType() {}

func (s *Scalar) String() string {
	return s.Type
}

// Object is a value with several fields
type Object struct {
	Name        string
	Description string
	Key         Resolver
	Fields      map[string]*Field
}

func (o *Object) isType() {}

func (o *Object) String() string {
	return o.Name
}

// List is a collection of other values
type List struct {
	Type Type
}

func (l *List) isType() {}

func (l *List) String() string {
	return fmt.Sprintf("[%s]", l.Type)
}

type InputObject struct {
	Name        string
	InputFields map[string]Type
}

func (io *InputObject) isType() {}

func (io *InputObject) String() string {
	return io.Name
}

// NonNull is a non-nullable other value
type NonNull struct {
	Type Type
}

func (n *NonNull) isType() {}

func (n *NonNull) String() string {
	return fmt.Sprintf("%s!", n.Type)
}

// Verify *Scalar, *Object, *List, *InputObject, and *NonNull implement Type
var _ Type = &Scalar{}
var _ Type = &Object{}
var _ Type = &List{}
var _ Type = &InputObject{}
var _ Type = &NonNull{}

// A Resolver calculates the value of a field of an object
type Resolver func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error)

// Field knows how to compute field values of an Object
//
// Fields are responsible for computing their value themselves.
type Field struct {
	Resolve        Resolver
	Type           Type
	Args           map[string]Type
	ParseArguments func(json interface{}) (interface{}, error)

	Expensive bool
}

type Schema struct {
	Query    Type
	Mutation Type
}

// SelectionSet represents a core GraphQL query
//
// A SelectionSet can contain multiple fields and multiple fragments. For
// example, the query
//
//     {
//       name
//       ... UserFragment
//       memberships {
//         organization { name }
//       }
//     }
//
// results in a root SelectionSet with two selections (name and memberships),
// and one fragment (UserFragment). The subselection `organization { name }`
// is stored in the memberships selection.
//
// Because GraphQL allows multiple fragments with the same name or alias,
// selections are stored in an array instead of a map.
type SelectionSet struct {
	Selections []*Selection
	Fragments  []*Fragment
	Complex    bool // Complex is true if a selection set has any nested selection sets
}

// A selection represents a part of a GraphQL query
//
// The selection
//
//     me: user(id: 166) { name }
//
// has name "user" (representing the source field to be queried), alias "me"
// (representing the name to be used in the output), args id: 166 (representing
// arguments passed to the source field to be queried), and subselection name
// representing the information to be queried from the resulting object.
type Selection struct {
	Name         string
	Alias        string
	Args         interface{}
	SelectionSet *SelectionSet
}

// A Fragment represents a reusable part of a GraphQL query
//
// The On part of a Fragment represents the type of source object for which
// this Fragment should be used. That is not currently implemented in this
// package.
type Fragment struct {
	On           string
	SelectionSet *SelectionSet
}

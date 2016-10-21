package graphql

import (
	"context"
	"fmt"
	"reflect"
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
	Name   string
	Key    *Field
	Fields map[string]*Field
}

func (o *Object) isType() {}

func (o *Object) String() string {
	return o.Name
}

// List is a collection of other values
type List struct {
	Type Type
}

func (s *List) isType() {}

func (s *List) String() string {
	return fmt.Sprintf("[%s]", s.Type)
}

// Verify *Scalar, *Object, and *List implement Type
var _ Type = &Scalar{}
var _ Type = &Object{}
var _ Type = &List{}

// Field knows how to compute field values of an Object
//
// Fields are responsible for computing their value themselves.
type Field struct {
	Name  string
	Index int

	// Resolve calculates the value of the field
	Resolve func(ctx context.Context, source, args interface{}, selectionSet *SelectionSet) (interface{}, error)

	// Type is the type of the field
	Type Type

	// ArgParser parses the args of the field
	ArgParser *ArgParser
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

type ArgParser struct {
	FromJSON func(interface{}, reflect.Value) error
	Type     reflect.Type
}

func (p *ArgParser) Parse(args interface{}) (interface{}, error) {
	if p == nil {
		if args, ok := args.(map[string]interface{}); !ok || len(args) != 0 {
			return nil, NewSafeError("unexpected args")
		}
		return nil, nil
	}

	parsed := reflect.New(p.Type).Elem()
	if err := p.FromJSON(args, parsed); err != nil {
		return nil, err
	}
	return parsed.Interface(), nil
}

// A Spec represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
//
// A Spec allows a developer to specify the type to be exposed, an optional key
// field (used for computing efficient deltas), and an optional set of methods
// that can be invoked on the exposed method.
//
// An example spec for a struct User could then look as follows:
//
//     type User struct {
//         Id   int64
//         Name string
//     }
//
//     var userSpec = Spec{
//         Type:    User{},
//         Key:     "id",
//         Methods: Methods{
//             "friends": func(u *User) []*User{
//                  return []*User{alice, bob},
//             },
//         }
//     }
//
type Spec struct {
	Type    interface{}
	Key     string
	Methods Methods
}

// A Methods map represents the set of methods exposed on a Spec.
//
// The name of each method should be the exposed GraphQL name of the method (ie
// "friends", not "Friends"), and the values should be functions that take the
// a value from the Spec's Type as a first argument. Because different methods
// have different types, the Methods map uses interface{} to store the methods.
type Methods map[string]interface{}

package schemabuilder

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
//
// A Object allows a developer to specify the type to be exposed, an optional key
// field (used for computing efficient deltas), and an optional set of methods
// that can be invoked on the exposed method.
//
// An example spec for a struct User could then look as follows:
//
//     type User struct {
//         Id   int64  `graphql:",key'`
//         Name string
//     }
//
//     var userObject = Object{
//         Type:    User{},
//         Methods: Methods{
//             "friends": func(u *User) []*User{
//                  return []*User{alice, bob},
//             },
//         }
//     }
//
type Object struct {
	Name        string // Optional, defaults to Type's name.
	Description string
	Type        interface{}
	Methods     Methods // Deprecated, use FieldFunc instead.
}

func (s *Object) FieldFunc(name string, f interface{}) {
	if s.Methods == nil {
		s.Methods = make(Methods)
	}
	s.Methods[name] = f
}

// A Methods map represents the set of methods exposed on a Object.
//
// The name of each method should be the exposed GraphQL name of the method (ie
// "friends", not "Friends"), and the values should be functions that take the
// a value from the Object's Type as a first argument. Because different methods
// have different types, the Methods map uses interface{} to store the methods.
type Methods map[string]interface{}

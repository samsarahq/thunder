package schemabuilder

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

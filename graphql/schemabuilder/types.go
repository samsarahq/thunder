package schemabuilder

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
)

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name            string // Optional, defaults to Type's name.
	Description     string
	Type            interface{}
	Methods         Methods // Deprecated, use FieldFunc instead.
	paginatedFields []paginationObject

	key string
}

type paginationObject struct {
	Name string
	Fn   interface{}
}

// FieldFuncOption is an interface for the variadic options that can be passed
// to a FieldFunc for configuring options on that function.
type FieldFuncOption func(*method)

// NonNullable is an option that can be passed to a FieldFunc to indicate that
// its return value is required, even if the return value is a pointer type.
func NonNullable(m *method) {
	m.MarkedNonNullable = true
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
	for _, option := range options {
		option(m)
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

// UnmarshalFunc unmarshal value into its known type.
// Look ar RegisterScalar function for an example.
type UnmarshalFunc func(value interface{}, dest reflect.Value) error

// RegisterScalar is used to register custom scalars.
//
// For example, to register a custom ID type,
// type ID struct {
// 		Value string
// }
//
// Implement JSON Marshalling
// func (id ID) MarshalJSON() ([]byte, error) {
//  return strconv.AppendQuote(nil, string(id.Value)), nil
// }
//
// Register unmarshal func
// func init() {
//	typ := reflect.TypeOf((*ID)(nil)).Elem()
//	if err := schemabuilder.RegisterScalar(typ, "ID", func(value interface{}, d reflect.Value) error {
//		v, ok := value.(string)
//		if !ok {
//			return errors.New("not a string type")
//		}
//
//		d.Field(0).SetString(v)
//		return nil
//	}); err != nil {
//		panic(err)
//	}
//}
func RegisterScalar(typ reflect.Type, name string, uf UnmarshalFunc) error {
	if typ.Kind() == reflect.Ptr {
		return errors.New("type should not be of pointer type")
	}

	if uf == nil {
		// Slow fail safe to avoid reflection code by package users
		if !reflect.PtrTo(typ).Implements(reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()) {
			return errors.New("either UnmarshalFunc should be provided or the provided type should implement json.Unmarshaler interface")
		}

		f, _ := reflect.PtrTo(typ).MethodByName("UnmarshalJSON")

		uf = func(value interface{}, dest reflect.Value) error {
			var x interface{}
			switch v := value.(type) {
			case []byte:
				x = v
			case string:
				x = []byte(v)
			case float64:
				x = []byte(strconv.FormatFloat(v, 'g', -1, 64))
			case int64:
				x = []byte(strconv.FormatInt(v, 10))
			case bool:
				if v {
					x = []byte{'t', 'r', 'u', 'e'}
				} else {
					x = []byte{'f', 'a', 'l', 's', 'e'}
				}
			default:
				return errors.New("unknown type")
			}

			if err := f.Func.Call([]reflect.Value{dest.Addr(), reflect.ValueOf(x)})[0].Interface(); err != nil {
				return err.(error)
			}

			return nil
		}
	}

	scalars[typ] = name
	scalarArgParsers[typ] = &argParser{
		FromJSON: uf,
	}

	return nil
}

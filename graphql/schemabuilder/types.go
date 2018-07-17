package schemabuilder

// A Object represents a Go type and set of methods to be converted into an
// Object in a GraphQL schema.
type Object struct {
	Name            string // Optional, defaults to Type's name.
	Description     string
	Type            interface{}
	Methods         Methods // Deprecated, use FieldFunc instead.
	PaginatedFields []PaginationObject

	key string
}

type PaginationObject struct {
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

func (s *Object) Key(f string) {
	s.key = f
}

type method struct {
	MarkedNonNullable bool
	Fn                interface{}
}

// A Methods map represents the set of methods exposed on a Object.
type Methods map[string]*method

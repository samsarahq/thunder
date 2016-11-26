// BuildSchema builds a graphql schema for a given server. Every type
// supported by the server should be exposed a method returning a
// graphql.Object{}.
//
// For example, a basic server type could look as follows:
//
//    type Server struct{}
//
//    type Foo struct{
//        Bar string
//    }
//    func (s *Server) Bar() graphql.Object {
//        return graphql.Object{
//            Type: Bar{},
//        }
//    }
//
//    type Query struct{}
//    func (s *Server) Query() graphql.Object {
//        return graphql.Object{
//            Type: Query{},
//            Methods: graphql.Methods{
//                "foo": func() (*Foo, error) {
//                    return &Foo{Bar: "bar"}, nil
//                },
//            },
//        }
//    }
//
//    type Mutation struct{}
//    func (s *Server) Mutation() graphql.Object {
//        return graphql.Object{
//            Type: Mutation{},
//        }
//    }
//
// BuildSchema supports a limited subset of types:
// - scalar types: ints, floats, bool, and string
// - optional scalar types: points to ints, floats, bool, and string
// - list types: slices of other supported types
// - object types: pointers to structs
//
// For object types, BuildSchema recursively builds a schema over the struct's
// fields and methods. All exported fields become fields in the schema, named by
// the privatized version of the name (FooBar -> fooBar), or by the name in the
// graphql tag if provided (`graphql:"foo"`).
//
// Methods also become fields in the schema. A method can optionally take both
// graphql arguments and a context argument. The graphql arguments must be
// specified in a struct with scalar fields. The context argument, if
// specified, must follow the graphql names. Both method names and argument names
// are privatized.
package schemabuilder

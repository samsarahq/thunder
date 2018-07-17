package main

import (
	"context"
	"net/http"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/graphiql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type Server struct {
}

type RoleType int32
type User struct {
	Id        int
	FirstName string
	LastName  string
	Role      RoleType
}

func (s *Server) registerUser(schema *schemabuilder.Schema) {
	object := schema.Object("User", User{})
	object.Key("Id")

	object.FieldFunc("fullName", func(u *User) string {
		return u.FirstName + " " + u.LastName
	})
}

type Args struct {
	Role *RoleType
}

func (s *Server) registerQuery(schema *schemabuilder.Schema) {
	object := schema.Query()

	var tmp RoleType
	schema.Enum(tmp, map[string]RoleType{
		"user":          RoleType(1),
		"manager":       RoleType(2),
		"administrator": RoleType(3),
	})

	userListRet := func(ctx context.Context, args Args) ([]*User, error) {
		return []*User{
			{
				Id:        1,
				FirstName: "Bob",
				LastName:  "Johnson",
				Role:      RoleType(1),
			},
			{
				Id:        2,
				FirstName: "Chloe",
				LastName:  "Kim",
				Role:      RoleType(1),
			},
		}, nil
	}

	object.FieldFunc("users", userListRet)

	object.PaginateFieldFunc("usersConnection", userListRet)

}

func (s *Server) registerMutation(schema *schemabuilder.Schema) {
	object := schema.Mutation()

	object.FieldFunc("echo", func(ctx context.Context, args struct{ Text string }) (string, error) {
		return args.Text, nil
	})

	object.FieldFunc("echoEnum", func(ctx context.Context, args struct {
		EnumField RoleType
	}) (RoleType, error) {
		return args.EnumField, nil
	})
}

func (s *Server) Schema() *graphql.Schema {
	schema := schemabuilder.NewSchema()

	s.registerUser(schema)
	s.registerQuery(schema)
	s.registerMutation(schema)

	return schema.MustBuild()
}

func main() {
	server := &Server{}
	graphqlSchema := server.Schema()
	introspection.AddIntrospectionToSchema(graphqlSchema)

	http.Handle("/graphql", graphql.Handler(graphqlSchema))
	http.Handle("/graphiql/", http.StripPrefix("/graphiql/", graphiql.Handler()))

	if err := http.ListenAndServe(":3030", nil); err != nil {
		panic(err)
	}
}

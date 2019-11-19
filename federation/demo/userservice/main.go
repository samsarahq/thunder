package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
)

type User struct {
	Id   int64
	Name string
}

type Address struct {
	City, Street string
}

type SearchResult struct {
	schemabuilder.Union
	*User
	*Address
}

func schema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func() []*User {
		return []*User{
			{
				Id:   1,
				Name: "Jim",
			},
			{
				Id:   2,
				Name: "Bob",
			},
		}
	})
	query.FieldFunc("searchUsers", func(args struct{ Query string }) []*SearchResult {
		return []*SearchResult{
			{
				User: &User{
					Id:   3,
					Name: args.Query + "-Person",
				},
			},
			{
				Address: &Address{
					City:   "Searchcity",
					Street: args.Query + " Square",
				},
			},
		}
	})

	user := schema.Object("User", User{})
	user.Federation(func(u *User) int64 {
		return u.Id
	})
	user.FieldFunc("address", func(ctx context.Context, in *User) (*Address, error) {
		return &Address{
			City:   in.Name + "city",
			Street: "Main street",
		}, nil
	})

	schema.Mutation().FieldFunc("register", func() {})

	return schema
}

func main() {
	server, err := federation.NewServer(schema().MustBuild())
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	thunderpb.RegisterExecutorServer(grpcServer, server)

	listener, err := net.Listen("tcp", ":1234")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

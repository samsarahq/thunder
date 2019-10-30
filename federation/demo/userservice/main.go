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

	user := schema.Object("User", User{})
	user.FieldFunc("address", func(ctx context.Context, in *User) (*Address, error) {
		return &Address{
			City:   in.Name + "city",
			Street: "Main street",
		}, nil
	})

	user.FieldFunc("federationKey", func(u *User) int64 {
		return u.Id
	})

	return schema
}

func main() {
	server, err := federation.NewServer(schema())
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

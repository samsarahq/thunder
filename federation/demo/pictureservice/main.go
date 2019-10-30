package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
)

type User struct {
	Id int64
}

type Picture struct {
	Url string
}

type Album struct {
	Pictures []*Picture
}

func schema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	federation := schema.Federation()
	federation.FieldFunc("User", func(args struct{ Keys []int64 }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key})
		}
		return users
	})

	user := schema.Object("User", User{})
	user.FieldFunc("picture", func(ctx context.Context, in *User) (*Picture, error) {
		return &Picture{
			Url: fmt.Sprintf("http://pictures/%d", in.Id),
		}, nil
	})
	user.FieldFunc("albums", func(in *User) ([]*Album, error) {
		return []*Album{
			{
				Pictures: []*Picture{
					{
						Url: fmt.Sprintf("http://pictures/%d", in.Id),
					},
				},
			},
		}, nil
	})

	return schema
}

func main() {
	server, err := federation.NewServer(schema().MustBuild())
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	thunderpb.RegisterExecutorServer(grpcServer, server)

	listener, err := net.Listen("tcp", ":1235")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

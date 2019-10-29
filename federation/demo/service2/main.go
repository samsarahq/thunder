package main

import (
	"context"
	"log"
	"net"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
	"google.golang.org/grpc"
)

type Foo struct {
	Name string
}

type Bar struct {
	Id int64
}

func schema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	schema.Query().FieldFunc("foosFromFederationKeys", func(args struct{ Keys []string }) []*Foo {
		foos := make([]*Foo, 0, len(args.Keys))
		for _, key := range args.Keys {
			foos = append(foos, &Foo{Name: key})
		}
		return foos
	})

	foo := schema.Object("foo", Foo{})

	// XXX: require schema.Key

	// XXX: how do we expose foo? ... flatten is annoying

	foo.FieldFunc("ok", func(ctx context.Context, in *Foo) (int, error) {
		return len(in.Name), nil
	})

	foo.FieldFunc("bar", func(in *Foo) *Bar {
		return &Bar{
			Id: int64(len(in.Name)*2 + 4),
		}
	})

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("federationKey", func(b *Bar) int64 {
		return b.Id
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

	listener, err := net.Listen("tcp", ":1235")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

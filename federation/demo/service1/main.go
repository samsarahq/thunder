package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/samsarahq/thunder/thunderpb"

	"github.com/samsarahq/thunder/federation"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

type Foo struct {
	Name string
}

type Bar struct {
	Id int64
}

func schema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("f", func() *Foo {
		return &Foo{
			Name: "jimbob",
		}
	})
	query.FieldFunc("fff", func() []*Foo {
		return []*Foo{
			{
				Name: "jimbo",
			},
			{
				Name: "bob",
			},
		}
	})

	foo := schema.Object("foo", Foo{})
	foo.BatchFieldFunc("hmm", func(ctx context.Context, in map[batch.Index]*Foo) (map[batch.Index]string, error) {
		out := make(map[batch.Index]string)
		for i, foo := range in {
			out[i] = foo.Name + "!!!"
		}
		return out, nil
	})
	foo.FieldFunc("federationKey", func(f *Foo) string {
		return f.Name
	})

	foo.FieldFunc("nest", func(f *Foo) *Foo {
		return f
	})

	schema.Query().FieldFunc("barsFromFederationKeys", func(args struct{ Keys []int64 }) []*Bar {
		bars := make([]*Bar, 0, len(args.Keys))
		for _, key := range args.Keys {
			bars = append(bars, &Bar{Id: key})
		}
		return bars
	})

	bar := schema.Object("bar", Bar{})
	bar.FieldFunc("baz", func(b *Bar) string {
		return fmt.Sprint(b.Id)
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

package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/thunderpb"
)

func main() {
	ctx := context.Background()

	execs := make(map[string]federation.ExecutorClient)
	for name, addr := range map[string]string{
		"service1": "localhost:1234",
		"service2": "localhost:1235",
	} {
		cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}
		execs[name] = &federation.GrpcExecutorClient{Client: thunderpb.NewExecutorClient(cc)}
	}

	e, err := federation.NewExecutor(ctx, execs)
	if err != nil {
		log.Fatal(err)
	}

	server, err := federation.NewGateway(e)
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	thunderpb.RegisterExecutorServer(grpcServer, server)

	listener, err := net.Listen("tcp", ":1236")
	if err != nil {
		log.Fatal(err)
	}

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal(err)
	}

	// http.Handle("/", http.FileServer(http.Dir(".")))
	// http.Handle("/graphql", federation.HTTPHandler(e))
	// http.ListenAndServe(":3030", nil)
}

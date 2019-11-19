package main

import (
	"context"
	"log"
	"net/http"

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

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.Handle("/graphql", federation.HTTPHandler(e))
	http.ListenAndServe(":3000", nil)
}

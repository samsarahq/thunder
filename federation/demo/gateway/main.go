package main

import (
	"net/http"

	"github.com/samsarahq/thunder/federation"
)

func main() {
	// ctx := context.Background()

	// execs := make(map[string]federation.ExecutorClient)
	// for name, addr := range map[string]string{
	// 	"service1": "localhost:1234",
	// 	"service2": "localhost:1235",
	// } {
	// 	cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	execs[name] = &federation.GrpcExecutorClient{Client: thunderpb.NewExecutorClient(cc)}
	// }

	// e, err := federation.NewExecutor(ctx, execs)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// gateway, err := federation.NewGateway(e)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.Handle("/graphql", federation.HTTPHandler())
	http.ListenAndServe(":3030", nil)
}

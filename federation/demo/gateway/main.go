package main

import (
	"context"
	"log"

	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/thunderpb"
)

func main() {
	ctx := context.Background()

	execs := make(map[string]thunderpb.ExecutorClient)
	for name, addr := range map[string]string{
		"service1": "localhost:1234",
		"service2": "localhost:1235",
	} {
		cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}
		execs[name] = thunderpb.NewExecutorClient(cc)
	}

	e, err := federation.NewExecutor(ctx, execs)
	if err != nil {
		log.Fatal(err)
	}

	oldQuery := graphql.MustParse(`
		{
			users {
				id
				name
				address { city street }
				picture { url }
			}
		}
	`, map[string]interface{}{})

	plan, err := e.Plan(oldQuery.SelectionSet)
	if err != nil {
		log.Fatal(err)
	}

	// XXX: have to deal with multiple plans here
	res, err := e.Execute(ctx, plan.After[0], nil)
	if err != nil {
		log.Fatal(err)
	}

	spew.Dump(res)
}

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"

	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/graphql"
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

	plan, err := e.Plan(oldQuery)
	if err != nil {
		log.Fatal(err)
	}

	// XXX: have to deal with multiple plans here
	res, err := e.Execute(ctx, plan)
	if err != nil {
		log.Fatal(err)
	}

	spew.Dump(res)

	http.Handle("/graphql", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		query, err := graphql.Parse(request.Query, request.Variables)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		plan, err := e.Plan(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := e.Execute(ctx, plan)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp = map[string]interface{}{
			"data": resp,
		}

		json.NewEncoder(w).Encode(resp)
	}))

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.ListenAndServe(":3000", nil)
}

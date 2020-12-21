package main

import (
	"context"
	"encoding/json"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/federation"
	"github.com/samsarahq/thunder/thunderpb"
	"github.com/samsarahq/thunder/graphql"
)


type GatewayExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *GatewayExecutorClient) Execute(ctx context.Context, req *graphql.Query) ([]byte, error) {
	marshaled, err := federation.MarshalQuery(req)
	if err != nil {
		return nil, err
	}
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

type Gateway struct {
	Executor *federation.Executor
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.ExecutorServer = &Gateway{}

func NewGateway(e *federation.Executor) (*Gateway, error) {
	return &Gateway{
		Executor: e,
	}, nil
}

// Execute unmarshals the protobuf query and executes it on the server
func (g *Gateway) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	query, err := federation.UnmarshalQuery(req.Query)
	if err != nil {
		return nil, err
	}
	res,_, err := g.Executor.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	resp := &thunderpb.ExecuteResponse{
		Result: bytes,
	}
	return resp, nil

}


type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, request *federation.QueryRequest) (*federation.QueryResponse, error) {
	// marshal query into a protobuf
	marshaled, err := federation.MarshalQuery(request.Query)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}
	// Make a request to the executor client with the query
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "executing query")
	}
	return &federation.QueryResponse{Result: resp.Result}, nil
}



func main() {
	ctx := context.Background()

	execs := make(map[string]federation.ExecutorClient)
	for name, addr := range map[string]string{
		"device": "localhost:1234",
		"safety": "localhost:1235",
	} {
		cc, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
		if err != nil {
			log.Fatal(err)
		}


		execs[name] = &federation.GrpcExecutorClient{Client: thunderpb.NewExecutorClient(cc)}
	}

	e, err := federation.NewExecutor(ctx, execs, &federation.SchemaSyncerConfig{SchemaSyncer: federation.NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	if err != nil {
		log.Fatal(err)
	}

	server, err := NewGateway(e)
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

}
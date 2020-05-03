package federation

import (
	"context"
	"encoding/json"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/thunderpb"
)

type GatewayExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *GatewayExecutorClient) Execute(ctx context.Context, req *graphql.Query) ([]byte, error) {
	// fmt.Println("MAOOPO")
	marshaled, err := marshalQuery(req)
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
	Executor *Executor
	// Client   thunderpb.ExecutorServer
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.ExecutorServer = &Gateway{}

func NewGateway(e *Executor) (*Gateway, error) {
	// introspection.AddIntrospectionToSchema(schema)

	return &Gateway{
		Executor: e,
	}, nil
}

// Execute unmarshals the protobuf query and executes it on the server
func (g *Gateway) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	// fmt.Println("YOOOO")
	query, err := unmarshalQuery(req.Query)
	if err != nil {
		return nil, err
	}
	res, err := g.Executor.Execute(ctx, query)
	if err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}

	// fmt.Println(result, err)
	resp := &thunderpb.ExecuteResponse{
		Result: bytes,
	}
	return resp, nil

}

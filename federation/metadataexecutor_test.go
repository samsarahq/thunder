package federation

import (
	"context"
	"testing"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/thunderpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

type SpecialMetadataExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *SpecialMetadataExecutorClient) Execute(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	// marshal query into a protobuf
	marshaled, err := MarshalQuery(request.Query)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}
	resp, err := c.Client.Execute(ctx, &thunderpb.ExecuteRequest{
		Query: marshaled,
	})
	if err != nil {
		return nil, oops.Wrapf(err, "executing query")
	}
	return &QueryResponse{
		Result:   resp.Result,
		Metadata: "respToken",
	}, nil
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.ExecutorServer = &CustomMetadataServer{}

type CustomMetadataServer struct {
	schema        *graphql.Schema
	localExecutor graphql.ExecutorRunner
}

func NewCustomMetadataServer(schema *graphql.Schema) (*CustomMetadataServer, error) {
	introspection.AddIntrospectionToSchema(schema)
	localExecutor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	return &CustomMetadataServer{
		schema:        schema,
		localExecutor: localExecutor,
	}, nil
}

// Execute adds the metadata to the ocntext and executes the request
func (s *CustomMetadataServer) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	for k, v := range md {
		ctx = context.WithValue(ctx, k, v[0])
	}

	return ExecuteRequest(ctx, req, s.schema, s.localExecutor)
}

func TestCustomMetadataExecutor(t *testing.T) {
	schema := createFederatedSchema(t)
	ctx := context.Background()
	execs := make(map[string]ExecutorClient)
	srv, err := NewCustomMetadataServer(schema.MustBuild())
	require.NoError(t, err)
	execs["s1"] = &SpecialMetadataExecutorClient{Client: srv}
	e, err := NewExecutor(ctx, execs, &CustomExecutorArgs{})
	require.NoError(t, err)
	md := metadata.Pairs("authtoken", "testToken")
	ctx = metadata.NewOutgoingContext(context.Background(), md)
	executeSuccesfulQuery(t, ctx, e, nil)
}

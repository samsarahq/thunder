package federation

import (
	"context"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/thunderpb"
)

type ExecutorClient interface {
	Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error)
}

func fetchSchema(ctx context.Context, e ExecutorClient) ([]byte, error) {
	query, err := graphql.Parse(introspection.IntrospectionQuery, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	selectionSet, err := marshalPbSelections(query.SelectionSet)
	if err != nil {
		return nil, err
	}

	out, err := e.Execute(ctx, &thunderpb.ExecuteRequest{
		Kind:         thunderpb.ExecuteRequest_QUERY,
		Name:         "introspection",
		SelectionSet: selectionSet,
	})
	if err != nil {
		return nil, err
	}

	return out.Result, nil
}

type GrpcExecutorClient struct {
	Client thunderpb.ExecutorClient
}

func (c *GrpcExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

type DirectExecutorClient struct {
	Client thunderpb.ExecutorServer
}

func (c *DirectExecutorClient) Execute(ctx context.Context, req *thunderpb.ExecuteRequest) (*thunderpb.ExecuteResponse, error) {
	return c.Client.Execute(ctx, req)
}

type ServiceVersion struct {
	Schema introspectionQueryResult
}

type ServiceConfig struct {
	Versions map[string]ServiceVersion
	Client   ExecutorClient
}

type ProxyConfig struct {
	Services map[string]ServiceConfig
}

type S3ProxyConfigLoader struct {
}

type LocalFileConfigLoader struct {
}

type ConfigPoller interface {
	Poll(ctx context.Context) (*ProxyConfig, error)
}

type Proxy struct {
}

func (p *Proxy) UpdateConfig(c *ProxyConfig) error {
	return nil
}

package federation

import (
	"context"
	"encoding/json"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

const IntrospectionClientName = "introspectionclient"

type IntrospectionClient struct {
	Schema   *graphql.Schema
	Executor graphql.ExecutorRunner
}

func NewIntrospectionClient(schema *graphql.Schema) *IntrospectionClient {
	executor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	return &IntrospectionClient{
		Schema:   schema,
		Executor: executor,
	}
}

func (c *IntrospectionClient) Execute(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	if err := graphql.PrepareQuery(ctx, c.Schema.Query, request.Query.SelectionSet); err != nil {
		return nil, err
	}

	res, err := c.Executor.Execute(ctx, c.Schema.Query, nil, request.Query)
	if err != nil {
		return nil, err
	}

	byts, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}

	return &QueryResponse{
		Result: byts,
	}, nil
}

// AddIntrospectionQueryToSchemaVersions will take in a map of service name to schema version
// to introspection query result, and append to it with another service called "introspectionclient"
// that is capable of only serving introspection queries. This will allow the federation executor to
// directly serve introspection queries via its built-in introspection client rather than relay its
// query to a subservice.
func AddIntrospectionQueryToSchemaVersions(schemas map[string]map[string]*IntrospectionQueryResult) (map[string]map[string]*IntrospectionQueryResult, error) {
	schema := schemabuilder.NewSchemaWithName("introspectionclient")
	schema.Query()
	schema.Mutation()

	gqlSchema := schema.MustBuild()
	introspection.AddIntrospectionToSchema(gqlSchema)

	res, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(gqlSchema))
	if err != nil {
		return nil, err
	}

	var introspectionClientSchema IntrospectionQueryResult
	if err := json.Unmarshal(res, &introspectionClientSchema); err != nil {
		return nil, err
	}

	schemas[IntrospectionClientName] = make(map[string]*IntrospectionQueryResult)
	schemas[IntrospectionClientName][""] = &introspectionClientSchema

	return schemas, nil
}

var _ ExecutorClient = &IntrospectionClient{}

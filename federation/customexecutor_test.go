package federation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/thunderpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SpecialExecutorClient struct {
	Client thunderpb.CustomExecutorServer
}

type Token struct {
	token string
}

func (c *SpecialExecutorClient) Execute(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	// 	// marshal query into a protobuf
	marshaled, err := MarshalQuery(request.Query)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}

	authToken := ""
	if request.Metadata != nil {
		token, ok := request.Metadata.(*Token)
		if !ok {
			return nil, oops.Errorf("incorrect token")
		}
		authToken = token.token
	}

	resp, err := c.Client.Execute(ctx, &thunderpb.CustomExecutorRequest{
		Request: &thunderpb.ExecuteRequest{
			Query: marshaled,
		},
		Token: authToken,
	})

	if err != nil {
		return nil, oops.Wrapf(err, "executing query")
	}
	return &QueryResponse{
		Result:   resp.Response.Result,
		Metadata: "respToken",
	}, nil
}

// Server must implement thunderpb.ExecutorServer.
var _ thunderpb.CustomExecutorServer = &CustomServer{}

type CustomServer struct {
	schema        *graphql.Schema
	localExecutor graphql.ExecutorRunner
}

func NewCustomExecutorServer(schema *graphql.Schema) (*CustomServer, error) {
	introspection.AddIntrospectionToSchema(schema)
	localExecutor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	return &CustomServer{
		schema:        schema,
		localExecutor: localExecutor,
	}, nil
}

// Execute unmarshals the protobuf query and executes it on the server
func (s *CustomServer) Execute(ctx context.Context, req *thunderpb.CustomExecutorRequest) (*thunderpb.CustomExecutorResponse, error) {
	ctx = context.WithValue(ctx, "authtoken", req.Token)
	resp, err := ExecuteRequest(ctx, req.Request, s.schema, s.localExecutor)
	if err != nil {
		return nil, err
	}
	return &thunderpb.CustomExecutorResponse{Response: resp, Token: "respToken"}, err
}

func createFederatedSchema(t *testing.T) *schemabuilder.Schema {
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("User", User{}, schemabuilder.RootObject)
	user.Key("id")
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		value, ok := ctx.Value("authtoken").(string)
		if ok && value == "testToken" {
			users := make([]*User, 0, 1)
			users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
			return users, nil
		}
		return nil, oops.Errorf("no token")

	})
	return s1
}

func executeSuccesfulQuery(t *testing.T, ctx context.Context, e *Executor, extraInformation interface{}) {
	query := `query Foo {
					users {
						id
					}
				}`
	res, optionalRespMetadata, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), extraInformation)
	require.NoError(t, err)
	output := `
	 	{
	 		"users":[
	 			{
	 				"__key":1,
	 				"id":1
	 			}
	 		]
	 	}`
	var expected interface{}
	err = json.Unmarshal([]byte(output), &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
	expectedoptionalRespMetadata := make([]interface{}, 1)
	expectedoptionalRespMetadata[0] = "respToken"
	assert.Equal(t, expectedoptionalRespMetadata, optionalRespMetadata)
}

func TestCustomExecutor(t *testing.T) {
	schema := createFederatedSchema(t)
	ctx := context.Background()
	execs := make(map[string]ExecutorClient)
	srv, err := NewCustomExecutorServer(schema.MustBuild())
	require.NoError(t, err)
	execs["s1"] = &SpecialExecutorClient{Client: srv}
	e, err := NewExecutor(ctx, execs, &CustomExecutorArgs{})
	require.NoError(t, err)
	executeSuccesfulQuery(t, ctx, e, &Token{token: "testToken"})
}

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

func (c *SpecialExecutorClient) Execute(ctx context.Context, req *graphql.Query, extraInformation interface{}) ([]byte, error) {
	// 	// marshal query into a protobuf
	marshaled, err := MarshalQuery(req)
	if err != nil {
		return nil, oops.Wrapf(err, "marshaling query")
	}

	authToken := ""
	if extraInformation != nil {
		token, ok := extraInformation.(*Token)
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
	return resp.Response.Result, nil
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
	ctx = context.WithValue(ctx, "authToken", req.Token)
	resp, err := ExecuteRequest(ctx, req.Request, s.schema, s.localExecutor)
	if err != nil {
		return nil, err
	}
	return &thunderpb.CustomExecutorResponse{Response: resp}, err
}

func TestCustomExecutor(t *testing.T) {
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("User", User{})
	user.Key("id")
	user.Federation(func(u *User) int64 {
		return u.Id
	})
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		value, ok := ctx.Value("authToken").(string)
		if ok && value == "testToken" {
			users := make([]*User, 0, 1)
			users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
			return users, nil
		}
		return nil, oops.Errorf("no token")

	})
	ctx := context.Background()
	execs := make(map[string]ExecutorClient)
	srv, err := NewCustomExecutorServer(s1.MustBuild())
	require.NoError(t, err)
	execs["s1"] = &SpecialExecutorClient{Client: srv}
	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	query := `query Foo {
					users {
						id
					}
				}`
	res, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), &Token{token: "testToken"})
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
}

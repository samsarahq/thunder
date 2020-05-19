package federation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeExecutors(schemas map[string]*schemabuilder.Schema) (map[string]ExecutorClient, error) {
	executors := make(map[string]ExecutorClient)

	for name, schema := range schemas {
		srv, err := NewServer(schema.MustBuild())
		if err != nil {
			return nil, err
		}
		executors[name] = &DirectExecutorClient{Client: srv}
	}

	return executors, nil
}

func createExecutorWithFederatedObjects() (*Executor, *schemabuilder.Schema, *schemabuilder.Schema, error) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id    int64
		OrgId int64
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("User", User{})
	user.Key("id")
	user.Federation(func(u *User) int64 {
		return u.Id
	})
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
		return users, nil
	})

	type Admin struct {
		Id         int64
		OrgId      int64
		SuperPower string
	}
	admin := s1.Object("Admin", Admin{})
	admin.Key("id")
	admin.Federation(func(a *Admin) int64 {
		return a.Id
	})
	s1.Query().FieldFunc("admins", func(ctx context.Context) ([]*Admin, error) {
		admins := make([]*Admin, 0, 1)
		admins = append(admins, &Admin{Id: int64(1), OrgId: int64(9086), SuperPower: "flying"})
		return admins, nil
	})

	type Everyone struct {
		schemabuilder.Union
		*User
		*Admin
	}
	s1.Query().FieldFunc("everyone", func(ctx context.Context) ([]*Everyone, error) {
		everyone := make([]*Everyone, 0, 2)
		everyone = append(everyone, &Everyone{Admin: &Admin{Id: int64(1), OrgId: int64(9086), SuperPower: "flying"}})
		everyone = append(everyone, &Everyone{User: &User{Id: int64(2), OrgId: int64(9086)}})
		return everyone, nil
	})

	// The second schema has a user with an email and a secret field
	type UserWithEmail struct {
		Id    int64
		OrgId int64
		Email string
	}
	s2 := schemabuilder.NewSchema()
	s2.Federation().FieldFunc("User", func(args struct{ Keys []int64 }) []*UserWithEmail {
		users := make([]*UserWithEmail, 0, len(args.Keys))
		users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
		return users
	})
	s2.Query().FieldFunc("secretUsers", func(ctx context.Context) ([]*UserWithEmail, error) {
		users := make([]*UserWithEmail, 0, 1)
		users = append(users, &UserWithEmail{Id: int64(1), OrgId: int64(1), Email: "test@gmail.com"})
		return users, nil
	})

	user2 := s2.Object("User", UserWithEmail{})
	user2.Key("id")
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithEmail) (string, error) {
		return "shhhhh", nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	e, err := NewExecutor(ctx, execs)
	return e, s1, s2, err
}

func getExpectedSchemaWithFederationInfo(t *testing.T, s1 *schemabuilder.Schema, s2 *schemabuilder.Schema) *SchemaWithFederationInfo {
	introspectionQueryReasult1 := extractSchema(t, s1.MustBuild())
	introspectionQueryReasult2 := extractSchema(t, s2.MustBuild())

	schemas := make(map[string]*introspectionQueryResult)
	schemas["s1"] = introspectionQueryReasult1
	schemas["s2"] = introspectionQueryReasult2

	// Add introspection schema for federeated client
	schemaWithFederationInfo, err := convertSchema(schemas)
	require.NoError(t, err)
	introspectionSchema := introspection.BareIntrospectionSchema(schemaWithFederationInfo.Schema)
	introspectionServer := &Server{schema: introspectionSchema}
	schemaWithIntrospection, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionServer.schema))
	require.NoError(t, err)
	var iq introspectionQueryResult
	err = json.Unmarshal(schemaWithIntrospection, &iq)
	require.NoError(t, err)
	schemas["introspection"] = &iq

	// Fetch federated schema from introspection query results
	schemaWithFederationInfo, err = convertSchema(schemas)
	require.NoError(t, err)
	return schemaWithFederationInfo
}

func TestMultipleExecutorGeneratedSchemas(t *testing.T) {
	e, s1, s2, err := createExecutorWithFederatedObjects()
	require.NoError(t, err)
	expectedSchemaWithFederationInfo := getExpectedSchemaWithFederationInfo(t, s1, s2)
	assert.Equal(t, getFieldServiceMaps(t, e.planner.schema), getFieldServiceMaps(t, expectedSchemaWithFederationInfo))
}

func runAndValidateQueryResults(t *testing.T, ctx context.Context, e *Executor, query string, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}))
	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
}

func TestExecutorQueriesFieldsOnOneService(t *testing.T) {
	e, _, _, err := createExecutorWithFederatedObjects()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields on schema s1",
			Query: `
				query Foo {
					users {
						id
					}
				}`,
			Output: `
				{
					"users":[
						{
							"__key":1,
							"id":1
						}
					]
				}`,
		},
		{
			Name: "query fields on schema s2",
			Query: `
				query Foo {
					secretUsers {
						id
						secret
					}
				}`,
			Output: `
				{
					"secretUsers":[
						{
							"__key":1,
							"id":1,
							"secret":"shhhhh"
						}
					]
				}`,
		},
		{
			Name: "query fields on schema s1 and s2",
			Query: `
				query Foo {
					admins {
						superPower
					}
					secretUsers {
						id
					}
				}`,
			Output: `
				{
					"admins":[
						{
							"__key":1,
							"superPower":"flying"
						}
					],
					"secretUsers":[
						{
							"__key":1,
							"id":1
						}
					]
				}`,
		},
		{
			Name: "multiple query fields on schema s1",
			Query: `
				query Foo {
					admins {
						superPower
					}
					users {
						id
					}
				}`,
			Output: `
				{
					"admins":[
						{
							"__key":1,
							"superPower":"flying"
						}
					],
					"users":[
						{
							"__key":1,
							"id":1
						}
					]
				}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			// Validates that we were able to execute the query on multiple
			// schemas and correctly stitch the results back together
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

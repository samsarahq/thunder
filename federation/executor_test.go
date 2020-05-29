package federation

import (
	"context"
	"encoding/json"
	"strings"
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

func createExecutor() (*Executor, error) {
	e, _, _, _, err := createExecutorWithFederatedObjects()
	return e, err
}
func createExecutorWithFederatedObjects() (*Executor, *schemabuilder.Schema, *schemabuilder.Schema, *schemabuilder.Schema, error) {
	// The first schema has a user object with an id and orgId
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
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086)})
		return users, nil
	})

	s1.Query().FieldFunc("usersWithArgs", func(args struct {
		Name string
	}) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(9086), Name: args.Name})
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

	// The second schema has a user with contact information
	type UserWithContactInfo struct {
		Id          int64
		OrgId       int64
		Email       string
		PhoneNumber string
	}
	s2 := schemabuilder.NewSchema()
	s2.Federation().FieldFunc("User", func(args struct{ Keys []int64 }) []*UserWithContactInfo {
		users := make([]*UserWithContactInfo, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &UserWithContactInfo{Id: key, Email: "yaaayeeeet@gmail.com", PhoneNumber: "555"})
		}
		return users
	})
	s2.Query().FieldFunc("secretUsers", func(ctx context.Context) ([]*UserWithContactInfo, error) {
		users := make([]*UserWithContactInfo, 0, 1)
		users = append(users, &UserWithContactInfo{Id: int64(1), OrgId: int64(1), Email: "test@gmail.com", PhoneNumber: "555"})
		return users, nil
	})

	user2 := s2.Object("User", UserWithContactInfo{})
	user2.Key("id")
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithContactInfo) (string, error) {
		return "shhhhh", nil
	})

	s2.Federation().FieldFunc("Admin", func(args struct{ Keys []int64 }) []*Admin {
		admins := make([]*Admin, 0, len(args.Keys))
		for _, key := range args.Keys {
			admins = append(admins, &Admin{Id: key, SuperPower: "flying"})
		}
		return admins
	})
	admin2 := s2.Object("Admin", Admin{})
	admin2.Key("id")
	admin2.FieldFunc("hiding", func(ctx context.Context, user *Admin) (bool, error) {
		return true, nil
	})

	type Driver struct {
		Id   int64
		Name string
	}
	driver := s1.Object("Driver", Driver{})
	driver.Key("id")
	driver.Federation(func(d *Driver) int64 {
		return d.Id
	})
	user.FieldFunc("driver", func(ctx context.Context, user *User) (*Driver, error) {
		return &Driver{Id: int64(1), Name: "bob"}, nil
	})

	s3 := schemabuilder.NewSchema()
	s3.Federation().FieldFunc("Driver", func(args struct{ Keys []int64 }) []*Driver {
		drivers := make([]*Driver, 0, len(args.Keys))
		for _, key := range args.Keys {
			drivers = append(drivers, &Driver{Id: key, Name: "bob"})
		}
		return drivers
	})
	driver3 := s3.Object("Driver", Driver{})
	driver3.Key("id")
	driver3.FieldFunc("isDriving", func(ctx context.Context, driver *Driver) (bool, error) {
		return true, nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
		"s3": s3,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	e, err := NewExecutor(ctx, execs)
	return e, s1, s2, s3, err
}

func getExpectedSchemaWithFederationInfo(t *testing.T, s1 *schemabuilder.Schema, s2 *schemabuilder.Schema, s3 *schemabuilder.Schema) *SchemaWithFederationInfo {
	introspectionQueryResult1 := extractSchema(t, s1.MustBuild())
	introspectionQueryResult2 := extractSchema(t, s2.MustBuild())
	introspectionQueryResult3 := extractSchema(t, s3.MustBuild())

	schemas := make(map[string]*introspectionQueryResult)
	schemas["s1"] = introspectionQueryResult1
	schemas["s2"] = introspectionQueryResult2
	schemas["s3"] = introspectionQueryResult3
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
	e, s1, s2, s3, err := createExecutorWithFederatedObjects()
	require.NoError(t, err)
	expectedSchemaWithFederationInfo := getExpectedSchemaWithFederationInfo(t, s1, s2, s3)
	assert.Equal(t, getFieldServiceMaps(t, e.planner.schema), getFieldServiceMaps(t, expectedSchemaWithFederationInfo))
}

func runAndValidateQueryResults(t *testing.T, ctx context.Context, e *Executor, query string, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), nil)
	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
}

func runAndValidateQueryError(t *testing.T, ctx context.Context, e *Executor, query string, out string, expectedError string) {
	_, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), nil)
	assert.True(t, strings.Contains(err.Error(), expectedError))
}

func TestExecutorQueriesFieldsOnOneService(t *testing.T) {
	e, err := createExecutor()
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

func TestExecutorQueriesFieldsOnOneServiceWithArgs(t *testing.T) {
	e, err := createExecutor()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields with args",
			Query: `
				query Foo {
					usersWithArgs(name: "foo") {
						id
						name
					}
				}`,
			Output: `
			{
				"usersWithArgs":[
					{
						"__key":1,
						"id":1,
						"name":"foo"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "query without necessary arguments",
			Query: `
				query Foo {
					usersWithArgs(foo: "foo") {
						id
						name
					}
				}`,
			Output:        "",
			Error:         true,
			ExpectedError: "error parsing args for \"usersWithArgs\": name: not a string",
		},
		{
			Name: "query without necessary arguments 2",
			Query: `
				query Foo {
					usersWithArgs {
						id
						name
					}
				}`,
			Output:        "",
			Error:         true,
			ExpectedError: "error parsing args for \"usersWithArgs\": name: not a string",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				// Validates that we were able to execute the query on multiple
				// schemas and correctly stitch the results back together
				runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
			} else {
				runAndValidateQueryError(t, ctx, e, testCase.Query, testCase.Output, testCase.ExpectedError)
			}

		})
	}
}

func TestExecutorQueriesFieldsOnMultipleServices(t *testing.T) {
	e, err := createExecutor()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields with args",
			Query: `
			query Foo {
				users {
					id
					email
				}
			}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"email":"yaaayeeeet@gmail.com",
						"id":1
					}
				]
			}`,
			Error: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				// Validates that we were able to execute the query on multiple
				// schemas and correctly stitch the results back together
				runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
			} else {
				runAndValidateQueryError(t, ctx, e, testCase.Query, testCase.Output, testCase.ExpectedError)
			}

		})
	}
}

func TestExecutorQueriesFieldsOnUnionTypes(t *testing.T) {
	e, err := createExecutor()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields with union type, admin and user, 2 server hops",
			Query: `
			query Foo {
				everyone {
					... on Admin {
						id
						hiding
						superPower

					}
					... on User {
						id
						email
					}
				}
			}`,
			Output: `
			{
				"everyone":[
					{
						"__key":1,
						"__typename":"Admin",
						"id":1,
						"hiding":true,
						"superPower":"flying"
					},
					{
						"__key":2,
						"__typename":"User",
						"id":2,
						"email":"yaaayeeeet@gmail.com"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "query fields with union type with nested object",
			Query: `
			query Foo {
				everyone {
					... on User {
						id
						driver {
							id
							isDriving
						}
					}
					... on Admin {
						id
					}
				}
			}`,
			Output: `
			{
				"everyone":[
					{
						"__key":1,
						"__typename":"Admin",
						"id":1
					},
					{
						"__key":2,
						"__typename":"User",
						"id":2,
						"driver": {
							"id":1,
							"__key":1,
							"isDriving":true
						}
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "query fields with union type for admin and user, 1 schema hop",
			Query: `
			query Foo {
				everyone {
					... on Admin {
						id
						hiding
					}
					... on User {
						id
					}
				}
			}`,
			Output: `
			{
				"everyone":[
					{
						"__key":1,
						"__typename":"Admin",
						"id":1,
						"hiding":true
					},
					{
						"__key":2,
						"__typename":"User",
						"id":2
					}
				]
			}`,
			Error: false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				// Validates that we were able to execute the query on multiple
				// schemas and correctly stitch the results back together
				runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
			} else {
				runAndValidateQueryError(t, ctx, e, testCase.Query, testCase.Output, testCase.ExpectedError)
			}

		})
	}
}

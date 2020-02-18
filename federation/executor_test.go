package federation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
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

func roundtripJson(t *testing.T, v interface{}) interface{} {
	bytes, err := json.Marshal(v)
	require.NoError(t, err)
	var r interface{}
	err = json.Unmarshal(bytes, &r)
	require.NoError(t, err)
	return r
}

func runAndValidateQueryResults(t *testing.T, ctx context.Context, e *Executor, query string, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}))
	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
}

func createExecutorWithFederatedObjects() (*Executor, error) {
	// The first schema has a user object with an id and orgId
	type User struct {
		Id    int64
		OrgId int64
	}
	s1 := schemabuilder.NewSchemaWithName("s1")
	user := s1.Object("User", User{})
	user.Key("id")

	type UserKeys struct {
		Id       int64
	}
	user.Federation(func(u *User) *UserKeys {
		return &UserKeys{Id: u.Id}
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
	admin.Federation(func(a *Admin) *UserKeys {
		return &UserKeys{Id: a.Id}
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
	s2 := schemabuilder.NewSchemaWithName("s2")
	s2.Federation().FederatedFieldFunc("User", func(args struct{ Keys []UserKeys }) []*UserWithEmail {
		users := make([]*UserWithEmail, 0, len(args.Keys))
		users = append(users, &UserWithEmail{Id: int64(1), Email: "yaaayeeeet@gmail.com"})
		return users
	})
	user2 := s2.Object("User", UserWithEmail{})
	user2.FieldFunc("secret", func(ctx context.Context, user *UserWithEmail) (string, error) {
		return "shhhhh", nil
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	if err != nil {
		return nil, err
	}

	return NewExecutor(ctx, execs)
}

func TestExecutorWithFederatedObject(t *testing.T) {
	e, err := createExecutorWithFederatedObjects()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		// {
		// 	Name: "query with admins, fields on one services",
		// 	Query: `
		// 		query Foo {
		// 			admins {
		// 				superPower
		// 			}
		// 		}`,
		// 	Output: `
		// 		{
		// 			"admins":[
		// 				{
		// 					"__key":1,
		// 					"superPower":"flying"
		// 				}
		// 			]
		// 		}`,
		// },
		{
			Name: "query with users, fields on both services",
			Query: `
				query Foo {
					users {
						id
						orgId
						email
						secret
					}
				}`,
			Output: `
				{
					"users":[
						{
							"__key":1,
							"email":"yaaayeeeet@gmail.com",
							"id":1,
							"orgId":9086,
							"secret":"shhhhh"
						}
					]
				}`,
		},
		{
			Name: "query with union type, multiple services",
			Query: `
				query Foo {
					everyone {
						... on Admin {
							id
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



func TestExecutorFailures(t *testing.T) {
	schema := schemabuilder.NewSchemaWithName("schema")
	schema.Query().FieldFunc("fail", func(ctx context.Context) (string, error) {
		return "", errors.New("somethings broken")
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)
	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx,t,e,`
		{
			fail
		}
	`,"executing sub plan: run on service: execute remotely: executing query: fail: somethings broken")
}

func assertExecuteError(ctx context.Context, t *testing.T, e *Executor, in, errMsg string) {
	_, err := e.Execute(ctx, graphql.MustParse(in, map[string]interface{}{}))
	require.EqualError(t, err, errMsg)
}


// TestExecutorCancelsOnFailure tests that a failing sub-query cancels
// other in-flight sub-queries.
func TestExecutorCancelsOnFailure(t *testing.T) {
	// s1 will fail, and expect s2 to be canceled in turn.
	s2started := make(chan struct{}, 0)
	s2canceled := make(chan struct{}, 0)

	s1 := schemabuilder.NewSchemaWithName("s1")
	s1.Query().FieldFunc("fail", func(ctx context.Context) (string, error) {
		<-s2started
		return "", errors.New("fail")
	})

	s2 := schemabuilder.NewSchemaWithName("s2")
	s2.Query().FieldFunc("wait", func(ctx context.Context) (string, error) {
		close(s2started)
		<-ctx.Done()
		assert.EqualError(t, ctx.Err(), "context canceled")
		close(s2canceled)
		return "", ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteError(ctx, t, e, `
		{
			fail
			wait
		}
	`, "executing sub plan: run on service: execute remotely: executing query: fail: fail")

	// Make sure s2 was actually canceled.
	<-s2canceled
}

func assertExecuteEqual(ctx context.Context, t *testing.T, e *Executor, in, out string) {
	res, err := e.Execute(ctx, graphql.MustParse(in, map[string]interface{}{}))
	require.NoError(t, err)

	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)

	assert.Equal(t, expected, roundtripJson(t, res))
}

// TestExecutorHasReactiveCache tests that a reactive.Cache works.
func TestExecutorHasReactiveCache(t *testing.T) {
	schema := schemabuilder.NewSchemaWithName("schema")
	schema.Query().FieldFunc("testCache", func(ctx context.Context) (string, error) {
		count := 0
		for i := 0; i < 2; i++ {
			v, err := reactive.Cache(ctx, "", func(ctx context.Context) (interface{}, error) {
				count++
				return "", nil
			})
			assert.NoError(t, err)
			assert.Equal(t, v, "")
		}
		assert.Equal(t, 1, count)
		return "", nil
	})

	ctx := context.Background()

	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs)
	require.NoError(t, err)
	assertExecuteEqual(ctx, t, e, `
		{
			testCache
		}
	`, `
		{"testCache": ""}
	`)
}
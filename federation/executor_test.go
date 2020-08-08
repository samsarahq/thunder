package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createExecutorWithFederatedUser() (*Executor, *schemabuilder.Schema, *schemabuilder.Schema, *schemabuilder.Schema, error) {
	/*
		Schema: s1
		Query {
			User {
				id: int64!
				orgId: int64!
				name: string!
				email       string
				phoneNumber string
				device: Device
				deviceWithArgs: Device
				__federation: User
			}
			users: [User]
			usersWithArgs: [User]
			Admin {
				id: int64!
				orgId: int64!
				superPower: string!
				hiding: bool
				__federation: Admin
			}
			admins: [Admin]
			everyone: [Admin || User]
			Device {
				id: int64!
				orgId: int64!
				isOn: bool
				__federation: Device
			}
		}
	*/
	type User struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}
	s1 := schemabuilder.NewSchemaWithName("s1")
	user := s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*User }) []*User {
		return args.Keys
	}))
	user.Key("id")
	type UserIds struct {
		Id    int64
		OrgId int64
	}
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})
	s1.Query().FieldFunc("usersWithArgs", func(args struct {
		Name string
	}) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: args.Name})
		return users, nil
	})

	type Admin struct {
		Id         int64
		OrgId      int64
		SuperPower string
	}
	admin := s1.Object("Admin", Admin{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*Admin }) []*Admin {
		return args.Keys
	}))
	admin.Key("id")
	admin.FieldFunc("hiding", func(ctx context.Context, user *Admin) (bool, error) {
		return true, nil
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
		everyone = append(everyone, &Everyone{User: &User{Id: int64(2), OrgId: int64(9086), Email: "email@gmail.com", PhoneNumber: "555-5555"}})
		return everyone, nil
	})

	type Device struct {
		Id    int64
		OrgId int64
		IsOn  bool
	}
	device := s1.Object("Device", Device{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*Device }) []*Device {
		return args.Keys
	}))
	device.Key("id")

	user.FieldFunc("device", func(ctx context.Context, user *User) (*Device, error) {
		return &Device{Id: int64(1), OrgId: int64(1), IsOn: true}, nil
	})

	user.FieldFunc("deviceWithArgs", func(ctx context.Context, user *User, args struct {
		Id int64
	}) (*Device, error) {
		return &Device{Id: args.Id, OrgId: user.OrgId}, nil
	})

	/*
		Schema: s2
		Query {
			Federation {
				User(keys: [UserKeysWithOrgId!]): [UserWithContactInfo]
			}
			User {
				id: int64!
				orgId: int64!
				name: string!
				email: string!
				phoneNumber: string!
				secret: string!
			}
		}
	*/
	type UserWithContactInfo struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}

	type UserKeysWithOrgId struct {
		Id    int64
		OrgId int64
	}
	s2 := schemabuilder.NewSchemaWithName("s2")
	userWithContactInfo := s2.Object("User", UserWithContactInfo{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserWithContactInfo }) []*UserWithContactInfo {
		return args.Keys
	}))
	userWithContactInfo.Key("id")
	userWithContactInfo.FieldFunc("secret", func(ctx context.Context, user *UserWithContactInfo) (string, error) {
		return "shhhhh", nil
	})

	/*
		Schema: s3
		Query {
			Federation {
				User(keys: [UserKeys!]): [UserWithAdminPrivelages]
			}
			User {
				__federation: User
				id: int64!
				orgId: int64!
				isAdmin: bool!
				privelages: string!
			}
			privelagedUsers: [User]
			Device {
				id: int64!
				orgId: int64!
				temp: in64!
			}
		}
	*/
	type UserWithAdminPrivelages struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}
	type UserKeys struct {
		Id int64
	}
	s3 := schemabuilder.NewSchemaWithName("s3")
	userWithAdminPrivelages := s3.Object("User", UserWithAdminPrivelages{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []UserKeys }) []*UserWithAdminPrivelages {
		users := make([]*UserWithAdminPrivelages, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &UserWithAdminPrivelages{Id: key.Id, OrgId: 0})
		}
		return users
	}))
	userWithAdminPrivelages.Key("id")
	userWithAdminPrivelages.FieldFunc("isAdmin", func(ctx context.Context, user *UserWithAdminPrivelages) (bool, error) {
		return true, nil
	})
	userWithAdminPrivelages.FieldFunc("privelages", func(ctx context.Context, user *UserWithAdminPrivelages) (string, error) {
		return "all", nil
	})

	type ShadowDevice struct {
		Id    int64
		OrgId int64
		IsOn  bool
	}
	deviceWithTemp := s3.Object("Device", ShadowDevice{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*ShadowDevice }) []*ShadowDevice {
		return args.Keys
	}))
	deviceWithTemp.Key("id")
	deviceWithTemp.FieldFunc("temp", func(ctx context.Context, device *ShadowDevice) int64 { return int64(70) })

	// Create the executor with all the schemas
	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
		"s3": s3,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}

	e, err := NewExecutor(ctx, execs, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	return e, s1, s2, s3, err
}

func runAndValidateQueryResults(t *testing.T, ctx context.Context, e *Executor, query string, out string) {
	res, _, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), nil)
	require.NoError(t, err)
	var expected interface{}
	err = json.Unmarshal([]byte(out), &expected)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
}

func runAndValidateQueryError(t *testing.T, ctx context.Context, e *Executor, query string, out string, expectedError string) {
	_, _, err := e.Execute(ctx, graphql.MustParse(query, map[string]interface{}{}), nil)
	assert.True(t, strings.Contains(err.Error(), expectedError))
}

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

func TestExecutorQueriesBasic(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields on one schema",
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
						},
						{
							"__key":2,
							"id":2
						}
					]
				}`,
		},
		{
			Name: "query fields on multiple schemas",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
						}
					}`,
			Output: `
					{
						"users":[
							{
								"__key":1,
								"id":1,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh"
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh"
							}
						]
					}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

func TestExecutorQueriesNestedObjects(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields with nested objects",
			Query: `
				query Foo {
					users {
						id
						device {
							id
						}
					}
				}`,
			Output: `
				{
					"users":[
						{
							"__key":1,
							"id":1,
							"device":{
								"__key":1,
								"id":1
							}
						},
						{
							"__key":2,
							"id":2,
							"device":{
								"__key":1,
								"id":1
							}
						}
					]
				}`,
		},
		{
			Name: "query fields with nested objects on multiple schemas",
			Query: `
				query Foo {
					users {
						id
						device {
							id
							isOn
							temp
						}
					}
				}`,
			Output: `
				{
					"users":[
						{
							"__key":1,
							"id":1,
							"device":{
								"__key":1,
								"id":1,
								"isOn":true,
								"temp":70
							}
						},
						{
							"__key":2,
							"id":2,
							"device":{
								"__key":1,
								"id":1,
								"isOn":true,
								"temp":70
							}
						}
					]
				}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

func TestExecutorQueriesWithArgs(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields root level with args",
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
		},
		{
			Name: "query fields multiple services with args",
			Query: `
				query Foo {
					usersWithArgs(name: "foo") {
						id
						name
						orgId
						deviceWithArgs(id:2) {
							id
							orgId
							temp
						}
					}
				}`,
			Output: `
			{
				"usersWithArgs":[
					{
						"__key":1,
						"id":1,
						"name":"foo",
						"orgId": 1,
						"deviceWithArgs" : {
								"__key": 2,
								"id": 2,
								"orgId": 1,
								"temp":70
						}
					}
				]
			}`,
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

func TestExecutorQueriesWithUnionTypes(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields with union type",
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
						device {
							id
						}
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
						"email":"email@gmail.com",
						"device": {
							"__key":1,
							"id":1
						}
					}
				]
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

func TestExecutorQueriesWithFragments(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields with inline fragments",
			Query: `
			query Foo {
				users {
					... on User {
						name
						isAdmin
					}
					... on User {
						id
						email
					}
				}
			}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id":1,
						"name":"testUser",
						"email":"email@gmail.com",
						"isAdmin":true
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"isAdmin":true
					}
				]
			}`,
		},
		{
			Name: "query fields with fragments",
			Query: `
			query Foo {
				users {
					id
					email
					...Bar
				}
			}
			fragment Bar on User {
				name
				isAdmin
			}
			`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id":1,
						"name":"testUser",
						"email":"email@gmail.com",
						"isAdmin":true
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"isAdmin":true
					}
				]
			}`,
		},
		{
			Name: "query fields with repeated fields and fragments",
			Query: `
			query Foo {
				users {
					... on User {
						name
						isAdmin
					}
					... on User {
						id
					}
					...Bar
				}
			}
			fragment Bar on User {
				id
				name
				email
			}
			`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id":1,
						"name":"testUser",
						"email":"email@gmail.com",
						"isAdmin":true
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"isAdmin":true
					}
				]
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

func TestExecutorQueriesWithBatching(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields with inline fragments",
			Query: `
			query Foo {
				users {
					... on User {
						name
						isAdmin
					}
					... on User {
						id
						email
					}
				}
			}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id":1,
						"name":"testUser",
						"email":"email@gmail.com",
						"isAdmin":true
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"isAdmin":true
					}
				]
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			runAndValidateQueryResults(t, ctx, e, testCase.Query, testCase.Output)
		})
	}
}

func TestExecutorWithInvalidFederationKeys(t *testing.T) {
	type User struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}
	s1 := schemabuilder.NewSchemaWithName("s1")
	type UserKey struct {
		Id int64
	}
	user := s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey }) []*User {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
	user.Key("id")
	type UserIds struct {
		Id    int64
		OrgId int64
	}
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})

	type UserWithExtraKey struct {
		Id          int64
		OrgId       int64
		Name        string
		UnkownField string
	}

	s2 := schemabuilder.NewSchemaWithName("s2")
	s2.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserWithExtraKey }) []*User {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))

	// Create the executor with all the schemas
	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	assert.NoError(t, err)

	_, err = NewExecutor(ctx, execs, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	assert.True(t, strings.Contains(err.Error(), "Invalid federation key unkownField"))
}

func createMutationExecutor() (map[string]ExecutorClient, error) {
	s1 := schemabuilder.NewSchemaWithName("s1")
	type User struct {
		Id   int64
		Name string
	}
	s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*User }) []*User {
		return args.Keys
	}))
	s1.Mutation().FieldFunc("newUser", func(ctx context.Context) (*User, error) {
		return &User{Id: int64(123), Name: "bob"}, nil
	})
	s2 := schemabuilder.NewSchemaWithName("s2")
	s2.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*User }) []*User {
		return args.Keys
	}))
	s2.Mutation().FieldFunc("newFakeUser", func(ctx context.Context) (*User, error) {
		return &User{Id: int64(234), Name: "fake"}, nil
	})
	return makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
}

func TestMutationExecutor(t *testing.T) {
	e, err := createMutationExecutor()
	require.NoError(t, err)
	ctx := context.Background()
	executor, err := NewExecutor(ctx, e, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, e, nil)})
	require.NoError(t, err)

	testCases := []struct {
		Name          string
		Query         string
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "query fields a succesful mutation",
			Query: `mutation NewUser {
				newUser {
					id
					name
				}
			}`,
			Output: `
			{
				"newUser":{
					"id":123,
					"name":"bob"
				}
			}`,
			Error: false,
		},
		{
			Name: "query fields multiple mutations",
			Query: `mutation NewUser {
				newUser {
					id
				}
				newFakeUser {
					id
				}
			}`,
			Output:        "",
			Error:         true,
			ExpectedError: "only support 1 mutation step to maintain ordering",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			if !testCase.Error {
				runAndValidateQueryResults(t, ctx, executor, testCase.Query, testCase.Output)
			} else {
				runAndValidateQueryError(t, ctx, executor, testCase.Query, testCase.Output, testCase.ExpectedError)
			}
		})
	}
}

func TestExecutorReturnsError(t *testing.T) {
	schema := schemabuilder.NewSchema()
	schema.Query().FieldFunc("fail", func(ctx context.Context) (string, error) {
		return "", errors.New("uh oh")
	})

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema": schema,
	})
	require.NoError(t, err)

	e, err := NewExecutor(ctx, execs, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	require.NoError(t, err)
	runAndValidateQueryError(t, ctx, e, `{ fail }`, ``, "executing query: fail: uh oh")
}

func TestExpectedFederatedObject(t *testing.T) {
	type User struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}
	s1 := schemabuilder.NewSchemaWithName("s1")
	type UserKey struct {
		Id int64
	}
	user := s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey }) []*User {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
	user.Key("id")
	type UserIds struct {
		Id    int64
		OrgId int64
	}
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})

	s2 := schemabuilder.NewSchemaWithName("s2")
	user2 := s2.Object("User", User{})
	user2.Key("id")
	user2.FieldFunc("isCool", func(ctx context.Context) (bool, error) {
		return true, nil
	})
	s2.Query().FieldFunc("users2", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})

	// Create the executor with all the schemas
	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	assert.NoError(t, err)

	_, err = NewExecutor(ctx, execs, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	fmt.Println(err)
	assert.True(t, strings.Contains(err.Error(), "Object User exists on another server and is not federated"))
}

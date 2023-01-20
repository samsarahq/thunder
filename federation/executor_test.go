package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
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
				_federation: User
			}
			users: [User]
			emptyusers: [User]
			usersWithArgs: [User]
			Admin {
				id: int64!
				orgId: int64!
				superPower: string!
				hiding: bool
				_federation: Admin
			}
			admins: [Admin]
			everyone: [Admin || User]
			Device {
				id: int64!
				orgId: int64!
				isOn: bool
				_federation: Device
			}
		}
	*/

	type Abilities struct {
		Teleportation bool
		BreathingFire []int
		Thunder       *bool
	}

	type Skills struct {
		Sports           []string
		Plumbing         *bool
		Eating           int
		Abilities        Abilities
		AbilitiesPointer *Abilities
		AllAbilities     []*Abilities
	}

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
	s1.Query().FieldFunc("emptyusers", func(ctx context.Context) ([]*User, error) {
		return []*User{}, nil
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
		Skills      Skills
	}

	type UserWithContactInfoKeys struct {
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
	userWithContactInfo := s2.Object("User", UserWithContactInfo{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserWithContactInfoKeys }) []*UserWithContactInfo {
		help := []*UserWithContactInfo{}
		plumbing := true
		thunder := true
		sampleAbilities := Abilities{Teleportation: true, BreathingFire: []int{1, 2, 3}}
		sampleAbilities2 := Abilities{Teleportation: true, BreathingFire: []int{4, 5, 6}, Thunder: &thunder}
		sampleSkills := Skills{Eating: 1, Plumbing: &plumbing, Sports: nil, Abilities: sampleAbilities, AbilitiesPointer: &sampleAbilities2, AllAbilities: []*Abilities{&sampleAbilities, &sampleAbilities2}}
		for _, i := range args.Keys {
			new := &UserWithContactInfo{
				Id:          i.Id,
				OrgId:       i.OrgId,
				Name:        i.Name,
				Email:       i.Email,
				PhoneNumber: i.PhoneNumber,
				Skills:      sampleSkills,
			}
			help = append(help, new)
		}
		return help
	}))
	s2.Object("Skills", Skills{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*Skills }) []*Skills {
		return args.Keys
	}))
	s2.Object("Abilities", Abilities{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*Abilities }) []*Abilities {
		return args.Keys
	}))

	userWithContactInfo.Key("id")
	userWithContactInfo.FieldFunc("secret", func(ctx context.Context, user *UserWithContactInfo) (string, error) {
		return "shhhhh", nil
	})

	userWithContactInfo.FieldFunc("skills", func(ctx context.Context, user *UserWithContactInfo) (Skills, error) {
		return user.Skills, nil
	})

	/*
		----------------------------
		Pagination Endpoints/Objects
		----------------------------
	*/
	type Item struct {
		Id         int64
		FilterText string
		Number     int64
		String     string
		Float      float64
	}

	type Args struct {
		Additional string
	}

	item := s2.Object("item", Item{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*Item }) []*Item {
		return args.Keys
	}))
	item.Key("id")

	userWithContactInfo.FieldFunc("testPagination", func(args Args) []Item {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}
	}, schemabuilder.Paginated)

	userWithContactInfo.FieldFunc("testPaginationWithFilter", func() []Item {
		return []Item{
			{Id: 1, FilterText: "can"},
			{Id: 2, FilterText: "man"},
			{Id: 3, FilterText: "cannot"},
			{Id: 4, FilterText: "soban"},
			{Id: 5, FilterText: "socan"},
			{Id: 6, FilterText: "crane"},
		}

	}, schemabuilder.Paginated,
		schemabuilder.FilterFunc("customFilter", func(searchTerm string) []string {
			return strings.Split(searchTerm, "")
		}, func(itemString string, searchTokens []string) bool {
			if len(itemString) < 1 {
				return false
			}

			// If the first character of the itemString matches the first character of a searchToken
			// return that it is a match
			isMatch := false
			for _, searchToken := range searchTokens {
				if len(searchToken) < 1 {
					continue
				}

				if itemString[0:1] == searchToken[0:1] {
					isMatch = true
					break
				}
			}
			return isMatch
		}),
		schemabuilder.BatchFilterField("foo",
			func(ctx context.Context, i map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(i))
				for i, item := range i {
					myMap[i] = item.FilterText
				}
				return myMap, nil
			},
		),
	)

	userWithContactInfo.FieldFunc("testPaginationWithSort", func() []Item {
		return []Item{
			{Id: 1, Number: 1, String: "1", Float: 1.0},
			{Id: 2, Number: 3, String: "3", Float: 3.0},
			{Id: 3, Number: 5, String: "5", Float: 5.0},
			{Id: 4, Number: 2, String: "2", Float: 2.0},
			{Id: 5, Number: 4, String: "4", Float: 4.0},
		}
	},
		schemabuilder.Paginated,
		schemabuilder.BatchSortField(
			"numbers", func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			}))

	/*
		----------------------------
		Pagination Endpoints/Objects
		----------------------------
	*/

	/*
		Schema: s3
		Query {
			Federation {
				User(keys: [UserKeys!]): [UserWithAdminPrivelages]
			}
			User {
				_federation: User
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
	d := json.NewDecoder(bytes.NewReader([]byte(out)))
	d.UseNumber()
	err = d.Decode(&expected)
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
		{
			Name: "query returning empty results on multiple schemas",
			Query: `
				query Foo {
					emptyusers {
						secret
					}
				}`,
			Output: `{
				"emptyusers": []
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

// This function checks that we are able to query for and receive an object that was passed between gql servers
// Specifically, it tests the functions in planner_helpers.go
func TestExecutorQueriesWithObjectKey(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields on multiple fields with object passthrough as well as nested object",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
							skills {
								sports
								plumbing
								eating
								abilities {
									teleportation
									breathingFire
								}
								abilitiesPointer{
									teleportation
									breathingFire
									thunder
								}
								allAbilities {
									teleportation
									breathingFire
								}
							}
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
								"secret": "shhhhh",
								"skills": {
									"sports": [],
									"plumbing": true,
									"eating": 1,
									"abilities": {
										"teleportation": true,
										"breathingFire": [1, 2, 3]
									},
									"abilitiesPointer": {
										"teleportation": true,
										"breathingFire": [4, 5, 6],
										"thunder": true
									},
									"allAbilities": [
										{
											"teleportation": true,
											"breathingFire": [1, 2, 3]
										},
										{
											"teleportation": true,
											"breathingFire": [4, 5, 6]
										}
									]
								}
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh",
								"skills": {
									"sports": [],
									"plumbing": true,
									"eating": 1,
									"abilities": {
										"teleportation": true,
										"breathingFire": [1, 2, 3]
									},
									"abilitiesPointer": {
										"teleportation": true,
										"breathingFire": [4, 5, 6],
										"thunder": true
									},
									"allAbilities": [
										{
											"teleportation": true,
											"breathingFire": [1, 2, 3]
										},
										{
											"teleportation": true,
											"breathingFire": [4, 5, 6]
										}
									]
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

func TestExecutorQueriesPagination(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name   string
		Query  string
		Output string
	}{
		{
			Name: "query fields on multiple schemas with pagination (first + after)",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
							testPagination(first: 1, after: "", additional: "jk") {
								totalCount
								edges {
									node {
										id
									}
									cursor
								}
								pageInfo {
									hasNextPage
									hasPrevPage
									startCursor
									endCursor
								}
							}
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
								"secret": "shhhhh",
								"testPagination":{
									"edges": [
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"id": 1
											}
										}
									],
									"pageInfo": {
										"endCursor": "MQ==",
										"hasNextPage": true,
										"hasPrevPage": false,
										"startCursor": "MQ=="
									},
									"totalCount": 5
								}
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh",
								"testPagination":{
									"edges": [
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"id": 1
											}
										}
									],
									"pageInfo": {
										"endCursor": "MQ==",
										"hasNextPage": true,
										"hasPrevPage": false,
										"startCursor": "MQ=="
									},
									"totalCount": 5
								}
							}
						]
					}`,
		},
		{
			Name: "query fields on multiple schemas with pagination (last + before)",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
							testPagination(last: 2, before: "", additional: "jk") {
								totalCount
								edges {
									node {
										id
									}
									cursor
								}
								pageInfo {
									hasNextPage
									hasPrevPage
									pages
									startCursor
									endCursor
								}
							}
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
								"secret": "shhhhh",
								"testPagination":{
									"edges": [
										{
											"cursor": "NA==",
											"node": {
												"__key": 4,
												"id": 4
											}
										},
										{
											"cursor": "NQ==",
											"node": {
												"__key": 5,
												"id": 5
											}
										}
									],
									"pageInfo": {
										"endCursor": "NQ==",
										"hasNextPage": false,
										"hasPrevPage": true,
										"pages": [
											"",
											"Mg==",
											"NA=="
										],
										"startCursor": "NA=="
									},
									"totalCount": 5
								}
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh",
								"testPagination":{
									"edges": [
										{
											"cursor": "NA==",
											"node": {
												"__key": 4,
												"id": 4
											}
										},
										{
											"cursor": "NQ==",
											"node": {
												"__key": 5,
												"id": 5
											}
										}
									],
									"pageInfo": {
										"endCursor": "NQ==",
										"hasNextPage": false,
										"hasPrevPage": true,
										"pages": [
											"",
											"Mg==",
											"NA=="
										],
										"startCursor": "NA=="
									},
									"totalCount": 5
								}
							}
						]
					}`,
		}, {
			Name: "query fields on multiple schemas with pagination with filter",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
							testPaginationWithFilter(filterText: "can", filterTextFields: ["foo"], first: 5, after: "", filterType: "customFilter") {
								totalCount
								edges {
									node {
										id
										filterText
									}
									cursor
								}
								pageInfo {
									hasNextPage
									hasPrevPage
									startCursor
									endCursor
									pages
								}
							}
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
								"secret": "shhhhh",
								"testPaginationWithFilter": {
									"edges": [
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"filterText": "can",
												"id": 1
											}
										},
										{
											"cursor": "Mw==",
											"node": {
												"__key": 3,
												"filterText": "cannot",
												"id": 3
											}
										},
										{
											"cursor": "Ng==",
											"node": {
												"__key": 6,
												"filterText": "crane",
												"id": 6
											}
										}
									],
									"pageInfo": {
										"endCursor": "Ng==",
										"hasNextPage": false,
										"hasPrevPage": false,
										"pages": [
											""
										],
										"startCursor": "MQ=="
									},
									"totalCount": 3
								}
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh",
								"testPaginationWithFilter": {
									"edges": [
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"filterText": "can",
												"id": 1
											}
										},
										{
											"cursor": "Mw==",
											"node": {
												"__key": 3,
												"filterText": "cannot",
												"id": 3
											}
										},
										{
											"cursor": "Ng==",
											"node": {
												"__key": 6,
												"filterText": "crane",
												"id": 6
											}
										}
									],
									"pageInfo": {
										"endCursor": "Ng==",
										"hasNextPage": false,
										"hasPrevPage": false,
										"pages": [
											""
										],
										"startCursor": "MQ=="
									},
									"totalCount": 3
								}
							}
						]
					}`,
		}, {
			Name: "query fields on multiple schemas with pagination with sort",
			Query: `
					query Foo {
						users {
							id
							email
							phoneNumber
							isAdmin
							secret
							testPaginationWithSort(sortBy: "numbers", sortOrder: "desc", first: 5, after: "") {
								totalCount
								edges {
									node {
										id
										number
									}
									cursor
								}
								pageInfo {
									hasNextPage
									hasPrevPage
									startCursor
									endCursor
									pages
								}
							}
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
								"secret": "shhhhh",
								"testPaginationWithSort": {
									"edges": [
										{
											"cursor": "Mw==",
											"node": {
												"__key": 3,
												"id": 3,
												"number": 5
											}
										},
										{
											"cursor": "NQ==",
											"node": {
												"__key": 5,
												"id": 5,
												"number": 4
											}
										},
										{
											"cursor": "Mg==",
											"node": {
												"__key": 2,
												"id": 2,
												"number": 3
											}
										},
										{
											"cursor": "NA==",
											"node": {
												"__key": 4,
												"id": 4,
												"number": 2
											}
										},
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"id": 1,
												"number": 1
											}
										}
									],
									"pageInfo": {
										"endCursor": "MQ==",
										"hasNextPage": false,
										"hasPrevPage": false,
										"pages": [
											""
										],
										"startCursor": "Mw=="
									},
									"totalCount": 5
								}
							},{
								"__key":2,
								"id":2,
								"email": "email@gmail.com",
								"phoneNumber": "555-5555",
								"isAdmin":true,
								"secret": "shhhhh",
								"testPaginationWithSort": {
									"edges": [
										{
											"cursor": "Mw==",
											"node": {
												"__key": 3,
												"id": 3,
												"number": 5
											}
										},
										{
											"cursor": "NQ==",
											"node": {
												"__key": 5,
												"id": 5,
												"number": 4
											}
										},
										{
											"cursor": "Mg==",
											"node": {
												"__key": 2,
												"id": 2,
												"number": 3
											}
										},
										{
											"cursor": "NA==",
											"node": {
												"__key": 4,
												"id": 4,
												"number": 2
											}
										},
										{
											"cursor": "MQ==",
											"node": {
												"__key": 1,
												"id": 1,
												"number": 1
											}
										}
									],
									"pageInfo": {
										"endCursor": "MQ==",
										"hasNextPage": false,
										"hasPrevPage": false,
										"pages": [
											""
										],
										"startCursor": "Mw=="
									},
									"totalCount": 5
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
		{
			Name: "query fields with repeated fields and fragments",
			Query: `
			query Foo {
				users {
					...Bar
				}
				users2: users {
					...Bar
				}
			}
			fragment Bar on User {
				id
				name
				email
				device {
					...DeviceInfo
				}
			}
			fragment DeviceInfo on Device {
				id
				temp
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
						"device":{
							"__key":1,
							"id":1,
							"temp":70
						}
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"device":{
							"__key":1,
							"id":1,
							"temp":70
						}
					}
				],
				"users2":[
					{
						"__key":1,
						"id":1,
						"name":"testUser",
						"email":"email@gmail.com",
						"device":{
							"__key":1,
							"id":1,
							"temp":70
						}
					},
					{
						"__key":2,
						"id":2,
						"name":"testUser2",
						"email":"email@gmail.com",
						"device":{
							"__key":1,
							"id":1,
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

func TestExecutorQueriesWithDirectives(t *testing.T) {
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
			Name: "query fields with nested objects on multiple schemas with directives",
			Query: `
				query Foo {
					users {
						id
						device @include (if: true) {
							id @skip(if: false)
							isOn @skip(if: true)
							temp @include(if: true)
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
								"temp":70
							}
						},
						{
							"__key":2,
							"id":2,
							"device":{
								"__key":1,
								"id":1,
								"temp":70
							}
						}
					]
				}`,
		},
		{
			Name: "directive top level selection true",
			Query: `
				query Foo {
					users @skip(if: true) {
						id
					}
					usersWithArgs(name: "foo") @include(if: true) {
						name
					}
				}`,
			Output: `
			{
				"usersWithArgs":[
					{
						"__key":1,
						"name":"foo"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "directive top level selection false",
			Query: `
				query Foo {
					users @include(if: false) {
						id
					}
					usersWithArgs(name: "foo") @skip(if: false) {
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
			Name: "directive nested selections",
			Query: `
				query Foo {
					users {
						id @skip(if: true)
						device @include(if: true){
							id
							isOn @skip(if: false)
							temp @include(if: false)
						}
					}
				}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"device":{
							"__key":1,
							"id":1,
							"isOn":true
						}
					},
					{
						"__key":2,
						"device":{
							"__key":1,
							"id":1,
							"isOn":true
						}
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "directive with fragments inline and repeated",
			Query: `
				query Foo {
					users {
						... on User @skip(if: true){
							name
							isAdmin
						}
						... on User @include(if: true){
							id
							email
						}
						...Bar @include(if: false)
						...Baz @skip(if: false)
					}
				}
				fragment Bar on User {
					orgId
				}
				fragment Baz on User {
					phoneNumber
				}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id":1,
						"email":"email@gmail.com",
						"phoneNumber": "555-5555"
					},
					{
						"__key":2,
						"id":2,
						"email":"email@gmail.com",
						"phoneNumber": "555-5555"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "directive with top level fragments",
			Query: `
				query Foo {
					...Bar @skip(if: true)
					...Baz @include(if: true)
				}
				fragment Bar on Query {
					users {
						id
					}
				}
				fragment Baz on Query {
					users {
						name
					}
				}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"name":"testUser"
					},
					{
						"__key":2,
						"name":"testUser2"
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "directive on both fragment and fragment selection",
			Query: `
				query Foo {
					users {
						...Bar @include(if: true)
					}
				}
				fragment Bar on User {
					id @include(if: true)
					phoneNumber @skip(if: true)
				}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"id": 1
					},
					{
						"__key":2,
						"id": 2
					}
				]
			}`,
			Error: false,
		},
		{
			Name: "directive on query fields with union type",
			Query: `
			query Foo {
				everyone {
					... on Admin @skip(if: false) {
						id @skip(if: true)
						superPower
					}
					... on User {
						id
						email
						device @include(if: false) {
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
						"superPower":"flying"
					},
					{
						"__key":2,
						"__typename":"User",
						"id":2,
						"email":"email@gmail.com"
					}
				]
			}`,
		},
		{
			Name: "directive missing argument",
			Query: `
				query Foo {
					users {
						...Bar @include(notif: true)
					}
				}
				fragment Bar on User {
					id
				}`,
			Output:        "",
			Error:         true,
			ExpectedError: "required argument in directive not provided: if",
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

func TestFederatedIntrospectionQuery(t *testing.T) {
	e, s1, s2, s3, err := createExecutorWithFederatedUser()
	require.NoError(t, err)

	// Reconstruct the expected introspection query result manually.
	schemaVersions := make(map[string]map[string]*IntrospectionQueryResult, 3)
	for i, s := range []*schemabuilder.Schema{s1, s2, s3} {
		iqBytes, err := introspection.ComputeSchemaJSON(*s)
		require.NoError(t, err)
		var iq IntrospectionQueryResult
		require.NoError(t, json.Unmarshal(iqBytes, &iq))
		schemaVersions[fmt.Sprintf("%d", i)] = map[string]*IntrospectionQueryResult{
			"": &iq,
		}
	}

	schemaVersions, err = AddIntrospectionQueryToSchemaVersions(schemaVersions)
	require.NoError(t, err)

	convertedSchema, err := ConvertVersionedSchemas(schemaVersions)
	require.NoError(t, err)

	schema := introspection.BareIntrospectionSchema(convertedSchema.Schema)
	schemaBytes, err := introspection.RunIntrospectionQuery(schema)
	require.NoError(t, err)

	var expectedIqRes IntrospectionQueryResult
	require.NoError(t, json.Unmarshal(schemaBytes, &expectedIqRes))

	// Run introspection query, expect the result to match.
	ctx := context.Background()
	res, _, err := e.Execute(ctx, graphql.MustParse(introspection.IntrospectionQuery, nil), nil)
	require.NoError(t, err)

	byts, err := json.Marshal(res)
	require.NoError(t, err)

	var actualIqRes IntrospectionQueryResult
	require.NoError(t, json.Unmarshal(byts, &actualIqRes))

	assert.Equal(t, expectedIqRes, actualIqRes)
}

func TestExecutorQueriesWithDirectivesWithVariables(t *testing.T) {
	e, _, _, _, err := createExecutorWithFederatedUser()
	require.NoError(t, err)
	testCases := []struct {
		Name          string
		Query         string
		Variables     map[string]interface{}
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "directive with skip variable true",
			Query: `
				query Foo {
					...Bar @skip(if: $something)
				}
				fragment Bar on Query {
					users {
						name
					}
				}`,
			Output: `
			{}`,
			Variables: map[string]interface{}{"something": true},
			Error:     false,
		},
		{
			Name: "directive with both variables false",
			Query: `
				query Foo {
					...Bar @skip(if: $something)
				}
				fragment Bar on Query {
					users {
						name
						id @include(if: $somethingElse)
					}
				}`,
			Output: `
			{
				"users":[
					{
						"__key":1,
						"name":"testUser"
					},
					{
						"__key":2,
						"name":"testUser2"
					}
				]
			}`,
			Variables: map[string]interface{}{"something": false, "somethingElse": false},
			Error:     false,
		},
		{
			Name: "directive with both variables false",
			Query: `
				query Foo {
					...Bar @skip(if: $something)
				}
				fragment Bar on Query {
					users {
						name
					}
				}`,
			Output:        "",
			Variables:     map[string]interface{}{"something": "wrong type"},
			Error:         true,
			ExpectedError: "expected type boolean, found type string in \"if\" argument",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				res, _, err := e.Execute(ctx, graphql.MustParse(testCase.Query, testCase.Variables), nil)
				require.NoError(t, err)
				var expected interface{}
				d := json.NewDecoder(bytes.NewReader([]byte(testCase.Output)))
				d.UseNumber()
				err = d.Decode(&expected)
				require.NoError(t, err)
				assert.Equal(t, expected, res)
			} else {
				_, _, err := e.Execute(ctx, graphql.MustParse(testCase.Query, testCase.Variables), nil)
				assert.True(t, strings.Contains(err.Error(), testCase.ExpectedError))
			}

		})
	}
}

func TestBasicFederatedObjectFetchAllFields(t *testing.T) {
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
	user2 := s2.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey }) []*User {
		users := make([]*User, 0, 2)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
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
	e, err := NewExecutor(ctx, execs, &SchemaSyncerConfig{SchemaSyncer: NewIntrospectionSchemaSyncer(ctx, execs, nil)})
	assert.NoError(t, err)

	testCases := []struct {
		Name          string
		Query         string
		Variables     map[string]interface{}
		Output        string
		Error         bool
		ExpectedError string
	}{
		{
			Name: "full query with all fields",
			Query: `
				query Foo {
					users {
						id
						name
						orgId
						email
						phoneNumber
						isCool
					}
					users2 {
						id
						name
						orgId
						email
						phoneNumber
						isCool
					}
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
							"isCool":true,
							"phoneNumber":"555-5555",
							"orgId":1
						},
						{
							"__key":2,
							"id":2,
							"name":"testUser2",
							"email":"email@gmail.com",
							"isCool":true,
							"phoneNumber":"555-5555",
							"orgId":2
						}
					],
					"users2":[
						{
							"__key":1,
							"id":1,
							"name":"testUser",
							"email":"email@gmail.com",
							"isCool":true,
							"phoneNumber":"555-5555",
							"orgId":1
						},
						{
							"__key":2,
							"id":2,
							"name":"testUser2",
							"email":"email@gmail.com",
							"isCool":true,
							"phoneNumber":"555-5555",
							"orgId":2
						}
					]
				}`,
			Variables: map[string]interface{}{"something": true},
			Error:     false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			if !testCase.Error {
				res, _, err := e.Execute(ctx, graphql.MustParse(testCase.Query, testCase.Variables), nil)
				require.NoError(t, err)
				var expected interface{}
				d := json.NewDecoder(bytes.NewReader([]byte(testCase.Output)))
				d.UseNumber()
				err = d.Decode(&expected)

				require.NoError(t, err)
				fmt.Println(expected)
				assert.Equal(t, expected, res)
			} else {
				_, _, err := e.Execute(ctx, graphql.MustParse(testCase.Query, testCase.Variables), nil)
				assert.True(t, strings.Contains(err.Error(), testCase.ExpectedError))
			}

		})
	}
}

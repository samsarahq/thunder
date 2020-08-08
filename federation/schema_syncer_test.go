package federation

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/require"
)

type FileSchemaSyncer struct {
	services        []string
	add             chan string // new URL channel
	currentSchema   []byte
	serviceSelector ServiceSelector
}

func newFileSchemaSyncer(ctx context.Context, services []string) *FileSchemaSyncer {
	ss := &FileSchemaSyncer{
		services: services,
		add:      make(chan string),
	}
	return ss
}

func (s *FileSchemaSyncer) FetchPlanner(ctx context.Context) (*Planner, error) {
	schemas := make(map[string]*IntrospectionQueryResult)
	for _, server := range s.services {
		schema, err := readFile(server)
		if err != nil {
			return nil, oops.Wrapf(err, "error reading file for server %s", server)
		}
		var iq IntrospectionQueryResult
		if err := json.Unmarshal(schema, &iq); err != nil {
			return nil, oops.Wrapf(err, "unmarshaling schema %s", server)
		}
		schemas[server] = &iq
	}

	types, err := convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	introspectionSchema := introspection.BareIntrospectionSchema(types.Schema)
	schema, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(introspectionSchema))
	if err != nil || schema == nil {
		return nil, oops.Wrapf(err, "error running introspection query")
	}

	var iq IntrospectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, oops.Wrapf(err, "unmarshaling introspection schema")
	}

	schemas["introspection"] = &iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	return NewPlanner(types, s.serviceSelector)
}

// WriteToFile will print any string of text to a file safely by
// checking for errors and syncing at the end.
func WriteToFile(filename string, data string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.WriteString(file, data)
	if err != nil {
		return err
	}
	return file.Sync()
}

func writeSchemaToFile(name string, data []byte) error {
	fileName := "./testdata/fileschemasyncer" + name
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.WriteString(file, string(data))
	if err != nil {
		return err
	}
	return file.Sync()
}

func readFile(name string) ([]byte, error) {
	fileName := "./testdata/fileschemasyncer" + name
	return ioutil.ReadFile(fileName)
}

func TestExecutorQueriesWithCustomSchemaSyncer(t *testing.T) {
	s1 := buildTestSchema1()
	s2 := buildTestSchema2()

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	// Write the schemas to a file
	services := []string{"s1", "s2"}
	for _, service := range services {
		schema, err := fetchSchema(ctx, execs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}

	// Creata file schema syncer that reads the schemas from the
	// written files and listens to updates if those change
	schemaSyncer := newFileSchemaSyncer(ctx, services)
	e, err := NewExecutor(ctx, execs, &SchemaSyncerConfig{
		SchemaSyncer:              schemaSyncer,
		SchemaSyncIntervalSeconds: func(ctx context.Context) int64 { return 1 },
	})
	require.NoError(t, err)

	// Test Case 1.
	query := `query Foo {
					s2root
					s1fff {
						s1hmm
					}
				}`
	expectedOutput := `{
					"s2root": "hello",
					"s1fff":[
						{
							"s1hmm":"jimbo!!!"
						},
						{
							"s1hmm":"bob!!!"
						}
					]
				}`

	// Run a federated query and ensure that it works
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
	time.Sleep(2 * time.Second)

	// Test Case 2.
	// Add a new field to schema2
	s2.Query().FieldFunc("syncerTest", func() string {
		return "hello"
	})

	newExecs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)

	// We need to do this to udpate the executor in our test
	// But when run locally it should already know about the new
	// field when the new service starts
	e.Executors = newExecs

	query2 := `query Foo {
		syncerTest
	}`
	expectedOutput2 := `{
		"syncerTest":"hello"
	}`

	// Since we havent written the new schema to the file, the merged schema and planner
	// dont know about the new field. We should see an error
	runAndValidateQueryError(t, ctx, e, query2, expectedOutput2, "unknown field syncerTest on typ Query")

	// Test case 3.
	// Writes the new schemas to the file
	for _, service := range services {
		schema, err := fetchSchema(ctx, newExecs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}

	// Sleep for 3 seconds to wait for the schema syncer to get the update
	time.Sleep(3 * time.Second)

	// 	Run the same query and validate that it works
	runAndValidateQueryResults(t, ctx, e, query2, expectedOutput2)

	// Test case 4.
	// Add the same fieldfunc to s1.
	s1.Query().FieldFunc("syncerTest", func() string {
		return "hello from s1"
	})
	newExecs, err = makeExecutors(map[string]*schemabuilder.Schema{
		"s1": s1,
		"s2": s2,
	})
	require.NoError(t, err)
	// We need to do this to udpate the executor in our test
	// But when run locally it should already know about the new
	// field when the new service starts
	e.Executors = newExecs
	// Writes the new schemas to the file
	for _, service := range services {
		schema, err := fetchSchema(ctx, newExecs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}
	// Sleep for 3 seconds to wait for the schema syncer to get the update
	time.Sleep(3 * time.Second)
	// Run the same query, the query should fail because the selection field has
	// more than 1 service associated without a selector.
	runAndValidateQueryError(t, ctx, e, query2, expectedOutput2, "is not in serviceSelector")

	// Test case 5.
	// Update the serviceSelector, syncerTestFunc to be resolved by service 1.
	schemaSyncer.serviceSelector = func(typeName string, fieldName string) string {
		if typeName == "Query" && fieldName == "syncerTest" {
			return "s1"
		}
		return ""
	}
	expectedOutput2 = `{
		"syncerTest":"hello from s1"
	}`
	// Sleep for 2 seconds to wait for the schema syncer to get the update
	time.Sleep(2 * time.Second)
	// Run the same query and validate that it works
	runAndValidateQueryResults(t, ctx, e, query2, expectedOutput2)
}

func TestOnlyShadowServiceKnowsAboutNewField(t *testing.T) {
	type User struct {
		Id          int64
		OrgId       int64
		Name        string
		Email       string
		PhoneNumber string
	}
	type UserKeys struct {
		Id    int64
		OrgId int64
	}

	s1 := schemabuilder.NewSchemaWithName("schema1")
	user := s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKeys }) []*User {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
	user.Key("id")
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})

	s2 := schemabuilder.NewSchemaWithName("schema2")
	userWithContactInfo := s2.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKeys }) []*User {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
	userWithContactInfo.FieldFunc("isCool", func(ctx context.Context) (bool, error) { return true, nil })

	ctx := context.Background()
	execs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})
	require.NoError(t, err)

	// Write the schemas to a file
	services := []string{"schema1", "schema2"}
	for _, service := range services {
		schema, err := fetchSchema(ctx, execs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}

	// Creata file schema syncer that reads the schemas from the
	// written files and listens to updates if those change
	schemaSyncer := newFileSchemaSyncer(ctx, services)
	e, err := NewExecutor(ctx, execs, &SchemaSyncerConfig{
		SchemaSyncer:              schemaSyncer,
		SchemaSyncIntervalSeconds: func(ctx context.Context) int64 { return 100 },
	})
	require.NoError(t, err)

	query := `query Foo {
					users {
						isCool
					}
				}`
	expectedOutput := `{
					"users":[
						{
							"__key":1,
							"isCool":true
						},
						{
							"__key":2,
							"isCool":true
						}
					]
				}`

	// Run a federated query and ensure that it works
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	type UserKeysNew struct {
		Id    int64
		OrgId int64
		Name  string `graphql:",optional"`
	}

	s2new := schemabuilder.NewSchemaWithName("schema2")
	userWithContactInfoNew := s2new.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKeysNew }) []*User {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users
	}))
	userWithContactInfoNew.FieldFunc("isCool", func(ctx context.Context) (bool, error) { return true, nil })

	newExecs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2new,
	})
	require.NoError(t, err)

	// The executor is updated. This mocsk the case where the federated executor
	// knows about a new field, but the gateway doesnt know about it yet
	// We want to fill it with a blank value until the gateway can correctly send the information
	e.Executors = newExecs

	// Run a federated query and ensure that it works
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

}

func updateSchemaFiles(t *testing.T, executors map[string]*schemabuilder.Schema) *Executor {
	ctx := context.Background()
	newExecs, err := makeExecutors(executors)
	services := []string{"schema1", "schema2"}
	for _, service := range services {
		schema, err := fetchSchema(ctx, newExecs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}
	schemaSyncer := newFileSchemaSyncer(ctx, services)
	e, err := NewExecutor(ctx, newExecs, &SchemaSyncerConfig{
		SchemaSyncer:              schemaSyncer,
		SchemaSyncIntervalSeconds: func(ctx context.Context) int64 { return 100 },
	})
	require.NoError(t, err)
	return e
}

func createSchemasWithFederatedUser(t *testing.T) (*schemabuilder.Schema, *schemabuilder.Schema, *Executor) {
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	type UserKey struct {
		Id    int64
		OrgId int64
	}
	s1 := schemabuilder.NewSchemaWithName("schema1")
	user := s1.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.OrgId, Name: "Bob"})
		}
		return users
	}))
	user.Key("id")
	user.FieldFunc("isCool2", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "bob"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "bob"})
		return users, nil
	})

	s2 := schemabuilder.NewSchemaWithName("schema2")
	user2 := s2.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.OrgId, Name: "Bob"})
		}
		return users
	}))
	user2.Key("id")
	user2.FieldFunc("isCool", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s2.Query().FieldFunc("users2", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "bob"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "bob"})
		return users, nil
	})
	e := updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})
	return s1, s2, e
}

func TestDeletingFederatedKey(t *testing.T) {
	ctx := context.Background()
	s1, s2, e := createSchemasWithFederatedUser(t)
	query := `query Foo {
					users {
						isCool
					}
					users2 {
						isCool2
					}
				}`
	expectedOutput := `{
					"users":[
						{
							"__key":1,
							"isCool":true
						},
						{
							"__key":2,
							"isCool":true
						}
					],
					"users2":[
						{
							"__key":1,
							"isCool2":true
						},
						{
							"__key":2,
							"isCool2":true
						}
					]
				}`

	// Run a federated query and ensure that it works
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 1: s2 no longer requests "orgId" as a key, gateway doesnt know
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	type UserKey2 struct {
		Id int64
	}
	s2New := schemabuilder.NewSchemaWithName("schema2")
	user2New := s2New.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey2 }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.Id, Name: "Bob"})
		}
		return users
	}))
	user2New.Key("id")
	user2New.FieldFunc("isCool", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s2New.Query().FieldFunc("users2", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1)})
		users = append(users, &User{Id: int64(2), OrgId: int64(2)})
		return users, nil
	})
	newExecs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2New,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 2: The s2 user schema no longer requests the field "orgId", gateway knows about the schema update
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2New,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	// Test 3: s1 user schema no longer reqyests the field "orgId", gateway doesnt know about the schema update
	s1New := schemabuilder.NewSchemaWithName("schema1")
	user2 := s1New.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey2 }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.Id, Name: "Bob"})
		}
		return users
	}))
	user2.Key("id")
	user2.FieldFunc("isCool2", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s1New.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1)})
		users = append(users, &User{Id: int64(2), OrgId: int64(2)})
		return users, nil
	})
	newExecs, err = makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 4: The s1 user schema no longer requests the field "orgId", gateway knows about the schema update
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	// Test 5: The s1 and s2 schema both no longer request "orgId", gateway doesnt know about it
	newExecs, err = makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2New,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 6: The s1 and s2 schema both no longer request "orgId", gateway knows about it
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2New,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
}

func TestAddingFederatedKey(t *testing.T) {
	ctx := context.Background()
	s1, s2, e := createSchemasWithFederatedUser(t)
	query := `query Foo {
					users {
						isCool
					}
					users2 {
						isCool2
					}
				}`
	expectedOutput := `{
					"users":[
						{
							"__key":1,
							"isCool":true
						},
						{
							"__key":2,
							"isCool":true
						}
					],
					"users2":[
						{
							"__key":1,
							"isCool2":true
						},
						{
							"__key":2,
							"isCool2":true
						}
					]
				}`

	// Run a federated query and ensure that it works
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 1: s2 requests new optional field "name" as a key, gateway doesnt know about the schema update
	type User struct {
		Id    int64
		OrgId int64
		Name  string
	}
	type UserKey2 struct {
		Id    int64
		OrgId int64
		Name  string `graphql:",optional"`
	}
	s2New := schemabuilder.NewSchemaWithName("schema2")
	user2New := s2New.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey2 }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.OrgId, Name: key.Name})
		}
		return users
	}))
	user2New.Key("id")
	user2New.FieldFunc("isCool", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s2New.Query().FieldFunc("users2", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "bob"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "bob"})
		return users, nil
	})
	newExecs, err := makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2New,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 2: s2 requests new optional field "name" as a key, gateway knows about the schema update
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2New,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	// Test 3: s1 requests new optional field "name" as a key, gateway doesnt know about the schema update
	s1New := schemabuilder.NewSchemaWithName("schema1")
	user2 := s1New.Object("User", User{}, schemabuilder.FetchObjectFromKeys(func(args struct{ Keys []*UserKey2 }) []*User {
		users := make([]*User, 0, len(args.Keys))
		for _, key := range args.Keys {
			users = append(users, &User{Id: key.Id, OrgId: key.Id, Name: "Bob"})
		}
		return users
	}))
	user2.Key("id")
	user2.FieldFunc("isCool2", func(ctx context.Context, user *User) (bool, error) { return true, nil })
	s1New.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "bob"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "bob"})
		return users, nil
	})
	newExecs, err = makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 4: The s1 user schema requests the new field "name", gateway knows about the schema update
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	// Test 5: The s1 and s2 schema both request "name", gateway doesnt know about it
	newExecs, err = makeExecutors(map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2New,
	})
	require.NoError(t, err)
	e.Executors = newExecs
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)

	// Test 6: The s1 and s2 schema both request "name", gateway knows about it
	updateSchemaFiles(t, map[string]*schemabuilder.Schema{
		"schema1": s1New,
		"schema2": s2New,
	})
	runAndValidateQueryResults(t, ctx, e, query, expectedOutput)
}

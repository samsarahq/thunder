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
	s1 := schemabuilder.NewSchemaWithName("schema1")
	user := s1.Object("User", User{}, schemabuilder.RootObject)
	user.Key("id")
	s1.Query().FieldFunc("users", func(ctx context.Context) ([]*User, error) {
		users := make([]*User, 0, 1)
		users = append(users, &User{Id: int64(1), OrgId: int64(1), Name: "testUser", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		users = append(users, &User{Id: int64(2), OrgId: int64(2), Name: "testUser2", Email: "email@gmail.com", PhoneNumber: "555-5555"})
		return users, nil
	})

	type UserWithContactInfo struct {
		Id    int64
		OrgId int64
		Name  string
	}

	s2 := schemabuilder.NewSchemaWithName("schema2")
	userWithContactInfo := s2.Object("User", UserWithContactInfo{}, schemabuilder.CustomShadowObject(func(args struct{ Keys []*UserWithContactInfo }) []*UserWithContactInfo {
		return args.Keys
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

	type UserWithContactInfoNew struct {
		Id    int64
		OrgId int64
		Name  string
		// When the gateway knows about email, it will fill it correctly
		Email string `graphql:",optional"`
		// This will never get filled correctly since the gateway doesnt know about this field
		UnknownField string `graphql:",optional"`
	}

	s2new := schemabuilder.NewSchemaWithName("schema2")
	userWithContactInfoNew := s2new.Object("User", UserWithContactInfoNew{}, schemabuilder.CustomShadowObject(func(args struct{ Keys []*UserWithContactInfoNew }) []*UserWithContactInfoNew {
		return args.Keys
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

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
	services      []string
	add           chan string // new URL channel
	ticker        *time.Ticker
	currentSchema []byte
}

func newFileSchemaSyncer(ctx context.Context, services []string) *FileSchemaSyncer {
	ss := &FileSchemaSyncer{
		services: services,
		ticker:   time.NewTicker(time.Second * 1),
		add:      make(chan string),
	}
	return ss
}

func (s *FileSchemaSyncer) FetchPlanner(ctx context.Context, optionalArgs interface{}) (*Planner, error) {
	schemas := make(map[string]*introspectionQueryResult)
	for _, server := range s.services {
		schema, err := readFile(server)
		if err != nil {
			return nil, oops.Wrapf(err, "error reading file for server %s", server)
		}
		var iq introspectionQueryResult
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

	var iq introspectionQueryResult
	if err := json.Unmarshal(schema, &iq); err != nil {
		return nil, oops.Wrapf(err, "unmarshaling introspection schema")
	}

	schemas["introspection"] = &iq
	types, err = convertSchema(schemas)
	if err != nil {
		return nil, oops.Wrapf(err, "converting schemas error")
	}

	return NewPlanner(types)
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
	e, err := NewExecutor(ctx, execs, &CustomExecutorArgs{
		SchemaSyncer:              schemaSyncer,
		SchemaSyncIntervalSeconds: func(ctx context.Context) int64 { return 5 },
	})
	require.NoError(t, err)

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
	time.Sleep(5 * time.Second)

	// Add a new field to schema2
	s2.Query().FieldFunc("s2root2", func() string {
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
		s2root2
	}`
	expectedOutput2 := `{
		"s2root2":"hello"
	}`

	// Since we havent written the new schema to the file, the merged schema and planner
	// dont know about the new field. We should see an error
	runAndValidateQueryError(t, ctx, e, query2, expectedOutput2, "unknown field s2root2 on typ Query")

	// Writes the new schemas to the file
	for _, service := range services {
		schema, err := fetchSchema(ctx, newExecs[service], nil)
		require.NoError(t, err)
		err = writeSchemaToFile(service, schema.Result)
		require.NoError(t, err)
	}

	// Sleep for 5 seconds to wait for the schema syncer to get the update
	time.Sleep(5 * time.Second)

	// 	Run the same query and validate that it works
	runAndValidateQueryResults(t, ctx, e, query2, expectedOutput2)

}

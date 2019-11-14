package federation

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/require"
)

func mustExtractSchema(schema *schemabuilder.Schema) introspectionQueryResult {
	bytes, err := introspection.ComputeSchemaJSON(*schema)
	if err != nil {
		log.Fatal(err)
	}
	var iq introspectionQueryResult
	if err := json.Unmarshal(bytes, &iq); err != nil {
		log.Fatal(err)
	}
	return iq
}

func mustExtractSchemas(schemas map[string]*schemabuilder.Schema) map[string]introspectionQueryResult {
	out := make(map[string]introspectionQueryResult)
	for k, v := range schemas {
		out[k] = mustExtractSchema(v)
	}
	return out
}

func TestBuildSchema(t *testing.T) {
	schemas := map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	}

	types, err := convertSchema(mustExtractSchemas(schemas))
	require.NoError(t, err)

	introspection.AddIntrospectionToSchema(types.Schema)
	out, err := introspection.RunIntrospectionQuery(types.Schema)
	require.NoError(t, err)

	var iq introspectionQueryResult
	err = json.Unmarshal(out, &iq)
	require.NoError(t, err)

	snapshotter := snapshotter.New(t)
	defer snapshotter.Verify()

	snapshotter.Snapshot("resulting schema", iq)
	// XXX: test field -> service mapping
}

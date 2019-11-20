package federation

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
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

// TestIncompatibleTypeKinds tests that incompatible types are caught by the
// schema merging.
func TestIncompatibleTypeKinds(t *testing.T) {
	// In s1, int is an object. In s2, int is a scalar. This is not allowed, as
	// different kinds can not be merged.
	type IntStruct struct{}
	s1 := schemabuilder.NewSchema()
	s1.Object("int", IntStruct{})
	s1.Query().FieldFunc("intStruct", func() IntStruct { return IntStruct{} })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("intScalar", func() int { return 0 })

	_, err := convertSchema(mustExtractSchemas(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	}))
	assert.EqualError(t, err, "conflicting kinds for typ int")
}

// TestIncompatibleInputTypesConflictingTypes tests that incompatible input types
// are caught by the schema merging.
func TestIncompatibleInputTypesConflictingTypes(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	{
		type InputStruct struct{ Foo string }
		s1.Query().FieldFunc("f", func(args struct{ I InputStruct }) string { return "" })
	}

	s2 := schemabuilder.NewSchema()
	{
		type InputStruct struct{ Foo int32 }
		s2.Query().FieldFunc("f", func(args struct{ I InputStruct }) string { return "" })
	}

	_, err := convertSchema(mustExtractSchemas(map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	}))
	assert.EqualError(t, err, "typ InputStruct_InputObject field foo has incompatible types string! and int32!: scalars must be identical")
}

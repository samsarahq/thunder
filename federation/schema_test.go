package federation

import (
	"encoding/json"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractSchema(t *testing.T, schema *graphql.Schema) introspectionQueryResult {
	bytes, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(schema))
	require.NoError(t, err)
	var iq introspectionQueryResult
	err = json.Unmarshal(bytes, &iq)
	require.NoError(t, err)
	return iq
}

func extractSchemas(t *testing.T, schemas map[string]*schemabuilder.Schema) map[string]introspectionQueryResult {
	out := make(map[string]introspectionQueryResult)
	for k, v := range schemas {
		out[k] = extractSchema(t, v.MustBuild())
	}
	return out
}

func extractConvertedSchemas(t *testing.T, schemas map[string]*schemabuilder.Schema) introspectionQueryResult {
	combined, err := convertSchema(extractSchemas(t, schemas))
	assert.NoError(t, err)
	return extractSchema(t, combined.Schema)
}

func TestBuildSchema(t *testing.T) {
	schemas := map[string]*schemabuilder.Schema{
		"schema1": buildTestSchema1(),
		"schema2": buildTestSchema2(),
	}

	types, err := convertSchema(extractSchemas(t, schemas))
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

	_, err := convertSchema(extractSchemas(t, map[string]*schemabuilder.Schema{
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

	_, err := convertSchema(extractSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	}))
	assert.EqualError(t, err, "service schema2 typ InputStruct_InputObject: field foo has incompatible types string! and int32!: scalars must be identical")
}

// TestIncompatibleInputTypesMissingNonNullField tests that incompatible input types
// are caught by the schema merging.
func TestIncompatibleInputTypesMissingNonNullField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	{
		type InputStruct struct{ Foo string }
		s1.Query().FieldFunc("f", func(args struct{ I InputStruct }) string { return "" })
	}

	s2 := schemabuilder.NewSchema()
	{
		type InputStruct struct{}
		s2.Query().FieldFunc("f", func(args struct{ I InputStruct }) string { return "" })
	}

	_, err := convertSchema(extractSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	}))
	assert.EqualError(t, err, "service schema2 typ InputStruct_InputObject: new field foo is non-null: string!")
}

// TestIncompatibleInputsConflictingTypes tests that incompatible input fields
// are caught by the schema merging.
func TestIncompatibleInputsConflictingTypes(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{ Foo string }) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{ Foo int32 }) string { return "" })

	_, err := convertSchema(extractSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	}))
	assert.EqualError(t, err, "service schema2 field f input: field map[foo:string!] has incompatible types string! and int32!: scalars must be identical")
}

// TestMergeNonNilNonNilField tests that a non-nil field combined with a non-nil
// field is non-nil in the combined schema.
func TestMergeNonNilNonNilField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{}) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{}) string { return "" })

	combined := extractConvertedSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{}) string { return "" })
	expected := extractSchema(t, s3.MustBuild())

	assert.Equal(t, expected, combined)
}

// TestMergeNonNilNilField tests that a non-nil field combined with a nilable
// field is nilable in the combined schema.
func TestMergeNonNilNilField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{}) *string { return nil })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{}) string { return "" })

	combined := extractConvertedSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{}) *string { return nil })
	expected := extractSchema(t, s3.MustBuild())

	assert.Equal(t, expected, combined)
}

// TestMergeNonNilNilArgument tests that a non-nil argument combined with a
// nilable field is not nilable in the combined schema.
func TestMergeNonNilNilArgument(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{ X string }) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{ X *string }) string { return "" })

	combined := extractConvertedSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": s1,
		"schema2": s2,
	})

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{ X string }) string { return "" })
	expected := extractSchema(t, s3.MustBuild())

	assert.Equal(t, expected, combined)
}

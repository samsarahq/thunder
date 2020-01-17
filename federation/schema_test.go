package federation

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/samsarahq/go/snapshotter"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractSchema(t *testing.T, schema *graphql.Schema) *introspectionQueryResult {
	bytes, err := introspection.RunIntrospectionQuery(introspection.BareIntrospectionSchema(schema))
	require.NoError(t, err)
	var iq introspectionQueryResult
	err = json.Unmarshal(bytes, &iq)
	require.NoError(t, err)
	return &iq
}

func extractSchemas(t *testing.T, schemas map[string]*schemabuilder.Schema) map[string]*introspectionQueryResult {
	out := make(map[string]*introspectionQueryResult)
	for k, v := range schemas {
		out[k] = extractSchema(t, v.MustBuild())
	}
	return out
}

func extractConvertedSchemas(t *testing.T, schemas map[string]*schemabuilder.Schema) *introspectionQueryResult {
	combined, err := convertSchema(extractSchemas(t, schemas))
	assert.NoError(t, err)
	return extractSchema(t, combined.Schema)
}

func assertSchemaUnionError(t *testing.T, a, b *schemabuilder.Schema, msg string) {
	_, err := convertSchema(extractSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": a,
		"schema2": b,
	}))
	assert.EqualError(t, err, msg)
}

func assertSchemaUnionEq(t *testing.T, a, b, c *schemabuilder.Schema) {
	combined := extractConvertedSchemas(t, map[string]*schemabuilder.Schema{
		"schema1": a,
		"schema2": b,
	})
	expected := extractSchema(t, c.MustBuild())
	assert.Equal(t, expected, combined)
}

func assertSchemaIntersectionEq(t *testing.T, a, b, c *schemabuilder.Schema) {
	combined, _ := extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"": map[string]*schemabuilder.Schema{
			"schema1": a,
			"schema2": b,
		},
	})
	expected := extractSchema(t, c.MustBuild())
	assert.Equal(t, expected, combined)
}

func getFieldServiceMaps(t *testing.T, s *SchemaWithFederationInfo) map[string][]string {
	types := make(map[graphql.Type]string)
	err := CollectTypes(s.Schema.Query, types)
	require.NoError(t, err)
	err = CollectTypes(s.Schema.Mutation, types)
	require.NoError(t, err)

	fieldServices := make(map[string][]string)

	for typ := range types {
		obj, ok := typ.(*graphql.Object)
		if !ok {
			continue
		}

		for fieldName, field := range obj.Fields {
			info, ok := s.Fields[field]
			if ok {
				services := make([]string, 0, len(info.Services))
				for service := range info.Services {
					services = append(services, service)
				}
				sort.Strings(services)
				name := fmt.Sprintf("%s.%s", obj.Name, fieldName)
				fieldServices[name] = services
			}
		}
	}

	return fieldServices
}

func extractConvertedVersionedSchemas(t *testing.T, schemas map[string]map[string]*schemabuilder.Schema) (*introspectionQueryResult, map[string][]string) {
	builtSchemas := make(serviceSchemas)
	for service, versions := range schemas {
		builtSchemas[service] = make(map[string]*introspectionQueryResult)
		for version, schema := range versions {
			builtSchemas[service][version] = extractSchema(t, schema.MustBuild())
		}
	}

	merged, err := convertVersionedSchemas(builtSchemas)
	require.NoError(t, err)

	return extractSchema(t, merged.Schema), getFieldServiceMaps(t, merged)
}

func TestBuildSchemaKitchenSink(t *testing.T) {
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

	assertSchemaUnionError(t, s1, s2, "can't merge type int: conflicting kinds OBJECT and SCALAR")
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

	assertSchemaUnionError(t, s1, s2, "can't merge type InputStruct_InputObject: merging input fields: field foo has incompatible types string! and int32!: types must be identical")
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

	assertSchemaUnionError(t, s1, s2, "can't merge type InputStruct_InputObject: merging input fields: new field foo is non-null: string!")
}

// TestIncompatibleInputsConflictingTypes tests that incompatible input fields
// are caught by the schema merging.
func TestIncompatibleInputsConflictingTypes(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{ Foo string }) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{ Foo int32 }) string { return "" })

	assertSchemaUnionError(t, s1, s2, "can't merge type Query: merging fields: field f has incompatible arguments: field foo has incompatible types string! and int32!: types must be identical")
}

// TestMergeNonNilNonNilField tests that a non-nil field combined with a non-nil
// field is non-nil in the combined schema.
func TestMergeNonNilNonNilField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{}) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{}) string { return "" })

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{}) string { return "" })

	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestMergeNonNilNilField tests that a non-nil field combined with a nilable
// field is nilable in the combined schema.
func TestMergeNonNilNilField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{}) *string { return nil })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{}) string { return "" })

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{}) *string { return nil })

	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestMergeNonNilNilArgument tests that a non-nil argument combined with a
// nilable field is not nilable in the combined schema.
func TestMergeNonNilNilArgument(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("f", func(args struct{ X string }) string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("f", func(args struct{ X *string }) string { return "" })

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("f", func(args struct{ X string }) string { return "" })

	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestIntersectionNewField tests that a new field is not included in the
// intersection of two schemas.
func TestIntersectionNewField(t *testing.T) {
	s1 := schemabuilder.NewSchema()
	s1.Query().FieldFunc("a", func() string { return "" })
	s1.Query().FieldFunc("b", func() string { return "" })

	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("a", func() string { return "" })

	s3 := schemabuilder.NewSchema()
	s3.Query().FieldFunc("a", func() string { return "" })

	assertSchemaIntersectionEq(t, s1, s2, s3)
}

// TestMergeUnionUnion tests that merging union types takes the union of their
// types.
func TestMergeUnionUnion(t *testing.T) {
	type Foo struct {
		Name string
	}
	type Bar struct {
		Id int64
	}
	type Baz struct {
		Address string
	}

	s1 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
			*Bar
		}
		s1.Query().FieldFunc("f", func() Union { return Union{} })
	}

	s2 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
			*Baz
		}
		s2.Query().FieldFunc("f", func() Union { return Union{} })
	}

	s3 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
			*Bar
			*Baz
		}
		s3.Query().FieldFunc("f", func() Union { return Union{} })
	}

	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestMergeUnionIntersection tests that merging union types takes the union of their
// types.
func TestMergeUnionIntersection(t *testing.T) {
	type Foo struct {
		Name string
	}
	type Bar struct {
		Id int64
	}
	type Baz struct {
		Address string
	}

	s1 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
			*Bar
		}
		s1.Query().FieldFunc("f", func() Union { return Union{} })
	}

	s2 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
			*Baz
		}
		s2.Query().FieldFunc("f", func() Union { return Union{} })
	}

	s3 := schemabuilder.NewSchema()
	{
		type Union struct {
			schemabuilder.Union
			*Foo
		}
		s3.Query().FieldFunc("f", func() Union { return Union{} })
	}

	assertSchemaIntersectionEq(t, s1, s2, s3)
}

// TestMergeEnumUnion tests that merging union types takes the union of their
// values.
func TestMergeEnumUnion(t *testing.T) {
	type Enum int32

	s1 := schemabuilder.NewSchema()
	{
		s1.Enum(Enum(0), map[string]Enum{
			"zero": 0,
			"one":  1,
		})
		s1.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	s2 := schemabuilder.NewSchema()
	{
		s2.Enum(Enum(0), map[string]Enum{
			"zero": 0,
			"two":  2,
		})
		s2.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	s3 := schemabuilder.NewSchema()
	{
		s3.Enum(Enum(0), map[string]Enum{
			"zero": 0,
			"one":  1,
			"two":  2,
		})
		s3.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestMergeEnumIntersection tests that merging union types takes the union of their
// values.
func TestMergeEnumIntersection(t *testing.T) {
	type Enum int32

	s1 := schemabuilder.NewSchema()
	{
		s1.Enum(Enum(0), map[string]Enum{
			"zero": 0,
			"one":  1,
		})
		s1.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	s2 := schemabuilder.NewSchema()
	{
		s2.Enum(Enum(0), map[string]Enum{
			"zero": 0,
			"two":  2,
		})
		s2.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	s3 := schemabuilder.NewSchema()
	{
		s3.Enum(Enum(0), map[string]Enum{
			"zero": 0,
		})
		s3.Query().FieldFunc("f", func() Enum { return Enum(1) })
	}

	assertSchemaIntersectionEq(t, s1, s2, s3)
}

// TestSchemaMoveField tests that moving a field from one service to another
// over the course of two deploys behaves sanely.
func TestSchemaMoveField(t *testing.T) {
	// In this test, a field "a" move from service s1 to service s2. At the
	// start, only s1 has the field. Then we deploy it to s2, let that deploy
	// stabilize, remove it from s1, and let that deploy stabilize.

	// s1old has the field.
	s1old := schemabuilder.NewSchema()
	s1old.Query().FieldFunc("a", func() string { return "" })

	// s1new does not.
	s1new := schemabuilder.NewSchema()

	// s2old does not have the field.
	s2old := schemabuilder.NewSchema()

	// s2new does.
	s2new := schemabuilder.NewSchema()
	s2new.Query().FieldFunc("a", func() string { return "" })

	// both has the field. This is what the final schema always should look
	// like.
	both := schemabuilder.NewSchema()
	both.Query().FieldFunc("a", func() string { return "" })
	expected := extractSchema(t, both.MustBuild())

	// Initial state, s1 has field a.
	combined, fieldServices := extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
		},
		"s2": map[string]*schemabuilder.Schema{
			"old": s2old,
		},
	})
	assert.Equal(t, expected, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
	}, fieldServices)

	// Add field to s2. Should still only route to s1.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
		},
		"s2": map[string]*schemabuilder.Schema{
			"old": s2old,
			"new": s2new,
		},
	})
	assert.Equal(t, expected, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
	}, fieldServices)

	// Let s2 stabilize. Should now route to both.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
		},
		"s2": map[string]*schemabuilder.Schema{
			"new": s2new,
		},
	})
	assert.Equal(t, expected, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1", "s2"},
	}, fieldServices)

	// Remove from s1. Should route to only s2.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"new": s2new,
		},
	})
	assert.Equal(t, expected, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s2"},
	}, fieldServices)

	// Let s1 stabilize. Should still route s2.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"new": s2new,
		},
	})
	assert.Equal(t, expected, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s2"},
	}, fieldServices)
}

// TestSchemaAddField tests adding a new field to a service.
func TestSchemaAddField(t *testing.T) {
	// In this test, a field "b" is added to service s1.

	// s1old has a field "a".
	s1old := schemabuilder.NewSchema()
	s1old.Query().FieldFunc("a", func() string { return "" })

	// s1new has both "a" and "b".
	s1new := schemabuilder.NewSchema()
	s1new.Query().FieldFunc("a", func() string { return "" })
	s1new.Query().FieldFunc("b", func() string { return "" })

	// s2 has a field "c".
	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("c", func() string { return "" })

	// before has "a" and "c".
	before := schemabuilder.NewSchema()
	before.Query().FieldFunc("a", func() string { return "" })
	before.Query().FieldFunc("c", func() string { return "" })
	builtBefore := extractSchema(t, before.MustBuild())

	// after has "a", "b", and "c".
	after := schemabuilder.NewSchema()
	after.Query().FieldFunc("a", func() string { return "" })
	after.Query().FieldFunc("b", func() string { return "" })
	after.Query().FieldFunc("c", func() string { return "" })
	builtAfter := extractSchema(t, after.MustBuild())

	// Initial state, s1 has field a and s2 has field c.
	combined, fieldServices := extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtBefore, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
		"Query.c": []string{"s2"},
	}, fieldServices)

	// Add field b to s1. Should not yet be exposed in schema.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtBefore, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
		"Query.c": []string{"s2"},
	}, fieldServices)

	// Let s1 stabilize. New field should now be visible.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtAfter, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
		"Query.b": []string{"s1"},
		"Query.c": []string{"s2"},
	}, fieldServices)
}

// TestSchemaRemoveField tests removing an existing field from a service.
func TestSchemaRemoveField(t *testing.T) {
	// In this test, a field "a" is removed from service s1.

	// s1old has a field "a".
	s1old := schemabuilder.NewSchema()
	s1old.Query().FieldFunc("a", func() string { return "" })

	// s1new has no field.
	s1new := schemabuilder.NewSchema()

	// s2 has a field "b".
	s2 := schemabuilder.NewSchema()
	s2.Query().FieldFunc("b", func() string { return "" })

	// before has "a" and "b".
	before := schemabuilder.NewSchema()
	before.Query().FieldFunc("a", func() string { return "" })
	before.Query().FieldFunc("b", func() string { return "" })
	builtBefore := extractSchema(t, before.MustBuild())

	// after has "b".
	after := schemabuilder.NewSchema()
	after.Query().FieldFunc("b", func() string { return "" })
	builtAfter := extractSchema(t, after.MustBuild())

	// Initial state, s1 has field a and s2 has field b.
	combined, fieldServices := extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtBefore, combined)
	assert.Equal(t, map[string][]string{
		"Query.a": []string{"s1"},
		"Query.b": []string{"s2"},
	}, fieldServices)

	// Remove field from s1. Should immediately disappear from schema.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"old": s1old,
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtAfter, combined)
	assert.Equal(t, map[string][]string{
		"Query.b": []string{"s2"},
	}, fieldServices)

	// Let s1 stabilize.
	combined, fieldServices = extractConvertedVersionedSchemas(t, map[string]map[string]*schemabuilder.Schema{
		"s1": map[string]*schemabuilder.Schema{
			"new": s1new,
		},
		"s2": map[string]*schemabuilder.Schema{
			"only": s2,
		},
	})
	assert.Equal(t, builtAfter, combined)
	assert.Equal(t, map[string][]string{
		"Query.b": []string{"s2"},
	}, fieldServices)
}

func buildTestSchemaFoo() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()
	query := schema.Query()
	foo := schema.Object("foo", Foo{})
	foo.Key("name")
	query.FieldFunc("name", func() *Foo {
		return &Foo{
			Name: "jim",
		}
	})

	mutation := schema.Mutation()
	mutation.FieldFunc("addName", func(args struct{ Name string }) *Foo {
		return &Foo{
			Name: args.Name,
		}
	})

	return schema
}

func buildTestSchemaBar() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()
	bar := schema.Object("bar", Bar{})
	bar.Key("id")
	query := schema.Query()
	query.FieldFunc("id", func() *Bar {
		return &Bar{
			Id: int64(1),
		}
	})
	mutation := schema.Mutation()
	mutation.FieldFunc("addId", func(args struct{ Id int64 }) *Bar {
		return &Bar{
			Id: args.Id,
		}
	})
	return schema
}

func TestBuildSchema(t *testing.T) {
	schemas := map[string]*schemabuilder.Schema{
		"schema1": buildTestSchemaFoo(),
		"schema2": buildTestSchemaBar(),
	}

	types, err := convertSchema(extractSchemas(t, schemas))
	require.NoError(t, err)

	introspection.AddIntrospectionToSchema(types.Schema)
	out, err := introspection.RunIntrospectionQuery(types.Schema)
	require.NoError(t, err)

	var iq introspectionQueryResult
	err = json.Unmarshal(out, &iq)
	require.NoError(t, err)
	fmt.Println(iq)
}

// TestSchemaUnionInputFields merges two schemas, getting the union of two sets of input fields
func TestSchemaUnionInputFields(t *testing.T) {
	type User struct {
		Id int64
	}
	s1 := schemabuilder.NewSchema()
	user := s1.Object("user", User{})
	user.Key("id")
	user.FieldFunc("id", func(user *User) int64 {
		return user.Id
	})
	q := s1.Query()
	q.FieldFunc("user", func(args struct {
		Id      int64
		Number1 *int64
	}) *User {
		return &User{
			Id: args.Id,
		}
	})

	s2 := schemabuilder.NewSchema()
	user2 := s2.Object("user", User{})
	user2.Key("id")
	user2.FieldFunc("id", func(user *User) int64 {
		return user.Id
	})
	q2 := s2.Query()
	q2.FieldFunc("user", func(args struct {
		Id      int64
		Number2 *int64
	}) *User {
		return &User{
			Id: args.Id,
		}
	})

	s3 := schemabuilder.NewSchema()
	user3 := s3.Object("user", User{})
	user3.Key("id")
	user3.FieldFunc("id", func(user *User) int64 {
		return user.Id
	})
	q3 := s3.Query()
	q3.FieldFunc("user", func(args struct {
		Id      int64
		Number1 *int64
		Number2 *int64
	}) *User {
		return &User{
			Id: int64(1),
		}
	})
	assertSchemaUnionEq(t, s1, s2, s3)
}

// TestSchemaIntersectionInputFields merges two schemas, getting the intersection of two sets of input fields
func TestSchemaIntersectionInputFields(t *testing.T) {
	type User struct {
		Id int64
	}

	s1 := schemabuilder.NewSchema()
	user := s1.Object("user", User{})
	user.Key("id")
	user.FieldFunc("id", func(user *User) int64 {
		return user.Id
	})
	q := s1.Query()
	q.FieldFunc("user", func(args struct{ Id int64 }) *User {
		return &User{
			Id: args.Id,
		}
	})

	s2 := schemabuilder.NewSchema()
	user2 := s2.Object("user", User{})
	user2.Key("id")
	user2.FieldFunc("id", func(user *User) int64 {
		return user.Id
	})
	q2 := s2.Query()
	q2.FieldFunc("user", func(args struct {
		Id     int64
		Number *int64
	}) *User {
		return &User{
			Id: args.Id,
		}
	})
	assertSchemaIntersectionEq(t, s1, s2, s1)
}

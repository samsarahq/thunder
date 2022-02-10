package schemabuilder_test

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/graphql/schemabuilder/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateEnumNamesFailSchema(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type DupedEnumType int32
	var dupeEnum1 DupedEnumType
	schema.Enum(dupeEnum1, map[string]DupedEnumType{
		"three": DupedEnumType(3),
		"two":   DupedEnumType(2),
		"one":   DupedEnumType(1),
	})

	var dupeEnum2 testdata.DupedEnumType
	schema.Enum(dupeEnum2, map[string]testdata.DupedEnumType{
		"four": testdata.DupedEnumType(4),
		"five": testdata.DupedEnumType(5),
		"six":  testdata.DupedEnumType(6),
	})

	_, err := schema.Build()
	require.Error(t, err)

	errStr := err.Error()
	require.Contains(t, errStr, "type name is duplicated")
	require.Contains(t, errStr, "DupedEnumType")
	require.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
	require.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
}

type User struct {
	Id int
}

func TestDuplicateScalarTypeNames(t *testing.T) {
	type DupedScalarType string
	type Wrapper struct {
		Val DupedScalarType
	}

	{
		dupedSchema := schemabuilder.NewSchema()
		query := dupedSchema.Query()
		query.FieldFunc("getScalar", func() DupedScalarType { return "" })
		query.FieldFunc("getOtherScalar", func() []*testdata.DupedScalarType { return nil })

		// Duplicate scalar type alias names are ok because
		// they are converted to their underlying scalar type.
		dupedSchema.MustBuild()
	}

	{
		dupedStructFieldSchema := schemabuilder.NewSchema()
		query := dupedStructFieldSchema.Query()

		query.FieldFunc("getScalarFieldWithScalarFieldArg",
			func(ctx context.Context, arg struct {
				Scalars []testdata.DupedScalarType
			}) Wrapper {
				return Wrapper{}
			})

		// Duplicate scalar type alias names in struct fields are ok because
		// they are converted to their underlying scalar type.
		dupedStructFieldSchema.MustBuild()
	}

	{
		dupedBatchSchema := schemabuilder.NewSchema()
		dupedBatchSchema.Query().FieldFunc("rootUser", func() User {
			return User{}
		})

		user := dupedBatchSchema.Object("user", User{})
		user.BatchFieldFunc("getDupedScalarBatch",
			func(ctx context.Context, batch map[batch.Index]User) map[batch.Index]DupedScalarType {
				return nil
			})
		user.BatchFieldFunc("getOtherDupedScalarBatch",
			func(ctx context.Context, batch map[batch.Index]User) map[batch.Index]testdata.DupedScalarType {
				return nil
			})

		// Duplicate scalar type alias names in batch return types are ok because
		// they are converted to their underlying scalar type.
		dupedBatchSchema.MustBuild()
	}
}

func TestDuplicateStructTypeNamesFailSchema(t *testing.T) {
	type DupedStructType struct {
		OtherField string
	}
	type Wrapper struct {
		Val DupedStructType
	}

	{
		dupedSchema := schemabuilder.NewSchema()
		query := dupedSchema.Query()
		query.FieldFunc("popularity", func(arg DupedStructType) []*testdata.DupedStructType { return nil })

		_, err := dupedSchema.Build()
		require.Error(t, err)

		errStr := err.Error()
		assert.Contains(t, errStr, "type name is duplicated")
		assert.Contains(t, errStr, "DupedStructType")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
	}

	{
		dupedStructFieldSchema := schemabuilder.NewSchema()
		query := dupedStructFieldSchema.Query()
		query.FieldFunc("getDataWithArgs", func(ctx context.Context, arg struct {
			Struct []*testdata.DupedStructType
		}) Wrapper {
			return Wrapper{}
		})

		_, err := dupedStructFieldSchema.Build()
		require.Error(t, err)

		errStr := err.Error()
		assert.Contains(t, errStr, "type name is duplicated")
		assert.Contains(t, errStr, "DupedStructType")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
	}

	{
		dupedBatchReturnSchema := schemabuilder.NewSchema()
		dupedBatchReturnSchema.Query().FieldFunc("rootUser", func() User { return User{} })

		user := dupedBatchReturnSchema.Object("user", User{})
		user.BatchFieldFunc("getDupedStructBatch",
			func(ctx context.Context, batch map[batch.Index]User) map[batch.Index]DupedStructType {
				return nil
			})
		user.BatchFieldFunc("getOtherDupedStructBatch",
			func(ctx context.Context, batch map[batch.Index]User) map[batch.Index]*testdata.DupedStructType {
				return nil
			})

		_, err := dupedBatchReturnSchema.Build()
		require.Error(t, err)

		errStr := err.Error()
		assert.Contains(t, errStr, "type name is duplicated")
		assert.Contains(t, errStr, "DupedStructType")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
	}

	{
		dupedBatchArgSchema := schemabuilder.NewSchema()
		dupedBatchArgSchema.Query().FieldFunc("rootStruct", func() DupedStructType {
			return DupedStructType{}
		})
		dupedBatchArgSchema.Query().FieldFunc("rootOtherStruct", func() testdata.DupedStructType {
			return testdata.DupedStructType{}
		})

		structType := dupedBatchArgSchema.Object("structType", DupedStructType{})
		structType.BatchFieldFunc("processDupedStructBatch",
			func(ctx context.Context, batch map[batch.Index]DupedStructType) map[batch.Index]int {
				return nil
			})

		otherStructType := dupedBatchArgSchema.Object("otherStructType", testdata.DupedStructType{})
		otherStructType.BatchFieldFunc("processOtherDupedStructBatch",
			func(ctx context.Context, batch map[batch.Index]*testdata.DupedStructType) map[batch.Index]int {
				return nil
			})

		_, err := dupedBatchArgSchema.Build()
		require.Error(t, err)

		errStr := err.Error()
		assert.Contains(t, errStr, "type name is duplicated")
		assert.Contains(t, errStr, "DupedStructType")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
	}
}

func TestDuplicateTypeNamesOfDifferentKinds(t *testing.T) {
	{
		dupedStructAndEnumSchema := schemabuilder.NewSchema()

		type Foo int32
		var foo Foo
		dupedStructAndEnumSchema.Enum(foo, map[string]Foo{
			"three": Foo(3),
			"two":   Foo(2),
			"one":   Foo(1),
		})

		query := dupedStructAndEnumSchema.Query()
		query.FieldFunc("foo", func(arg struct{ Foo []*testdata.Foo }) Foo { return 1 })

		_, err := dupedStructAndEnumSchema.Build()
		require.Error(t, err)

		errStr := err.Error()
		assert.Contains(t, errStr, "type name is duplicated")
		assert.Contains(t, errStr, "Foo")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder_test")
		assert.Contains(t, errStr, "github.com/samsarahq/thunder/graphql/schemabuilder/testdata")
	}

	{
		dupedEnumAndScalarSchema := schemabuilder.NewSchema()

		type Bar int32
		var bar Bar
		dupedEnumAndScalarSchema.Enum(bar, map[string]Bar{
			"three": Bar(3),
			"two":   Bar(2),
			"one":   Bar(1),
		})

		query := dupedEnumAndScalarSchema.Query()
		query.FieldFunc("bar", func(arg struct {
			bar Bar
		}) []*testdata.Bar {
			return nil
		})

		// int32 Bar type is converted to int32 so there is no conflict with enum type Bar.
		_, err := dupedEnumAndScalarSchema.Build()
		require.NoError(t, err)
	}

	{
		dupedStructAndScalarSchema := schemabuilder.NewSchema()

		type Foo int32

		query := dupedStructAndScalarSchema.Query()
		query.FieldFunc("foo", func(arg struct{ Foo []*testdata.Foo }) Foo { return 1 })

		// int32 Foo type is converted to int32 so there is no conflict with struct type Foo.
		_, err := dupedStructAndScalarSchema.Build()
		require.NoError(t, err)
	}
}

func TestRecursiveType(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type Tree struct {
		Name     string
		Children []*Tree
	}

	schema.Query().FieldFunc("tree", func(ctx context.Context, tree Tree) int {
		return 0
	})

	schema.MustBuild()
}

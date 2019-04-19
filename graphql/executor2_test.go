package graphql_test

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
)

func TestSimple(t *testing.T) {
	type foo struct {
		Name   string
		Filter string
	}

	builder := schemabuilder.NewSchema()
	builder.Query().FieldFunc("foo", func(ctx context.Context, args struct {
		Filter string
	}) []foo {
		return []foo{
			{Name: "foo1", Filter: args.Filter},
			{Name: "foo2", Filter: args.Filter},
		}
	})

	schema := builder.MustBuild()

	q := graphql.MustParse(`{ foo(filter: "") { name, filter } }`, nil)
	if err := graphql.PrepareQuery(schema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
}

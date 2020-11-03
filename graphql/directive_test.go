package graphql_test

import (
	"context"
	"testing"

	"github.com/northvolt/thunder/graphql"
	"github.com/northvolt/thunder/graphql/schemabuilder"
	"github.com/northvolt/thunder/internal/testgraphql"
	"github.com/stretchr/testify/assert"
)

func buildSchema() *graphql.Schema {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	item := schema.Object("item", Item{})
	item.Key("id")
	item.FieldFunc("name", func(ctx context.Context, item Item) (string, error) {
		return string(item.Id), nil
	})
	item.FieldFunc("number", func(ctx context.Context, item Item) (string, error) {
		return string(item.Number), nil
	})
	query.FieldFunc("items", func(ctx context.Context) ([]Item, error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1, Number: 11}
		retList[1] = Item{Id: 2, Number: 12}
		retList[2] = Item{Id: 3, Number: 13}
		retList[3] = Item{Id: 4, Number: 14}
		retList[4] = Item{Id: 5, Number: 15}
		return retList, nil
	})
	return schema.MustBuild()
}

func TestSkipDirectives(t *testing.T) {
	builtSchema := buildSchema()
	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Directive skip top level selection", `{
		items @skip(if: true){
			id
			name
		}
	}`)

	snap.SnapshotQuery("Directive don't skip top level selection", `{
		items @skip(if: false){
			id
			name
		}
	}`)

	snap.SnapshotQuery("Directive skip nested selection", `{
		items {
			id    @skip(if: true)
			name  @skip(if: false)
		}
	}`)

}

func TestIncludeDirectives(t *testing.T) {
	builtSchema := buildSchema()
	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Directive include top level selection", `{
		items @include(if: true){
			id
			name
		}
	}`)

	snap.SnapshotQuery("Directive don't include top level selection", `{
		items @include(if: false){
			id
			name
		}
	}`)

	snap.SnapshotQuery("Directive include nested selection", `{
		items {
			id    @include(if: true)
			name  @include(if: false)
		}
	}`)

}

func TestDirectivesWithFragments(t *testing.T) {
	builtSchema := buildSchema()
	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Directive with fragment on top level, skip true", `query x {
			...X @skip(if: true)
		}
		fragment X on Query {
			items {
				name
			}
		}`)

	snap.SnapshotQuery("Directive with fragment nested, skip true", `query x {
			items {
				id
				...X @skip(if: true)
			}
		}
		fragment X on Item {
			name
		}`)

	snap.SnapshotQuery("Directive on fragment selection, skip true", `query x {
		items {
			id
			...X
		}
	}
	fragment X on Item {
		name @skip(if: true)
	}`)

	snap.SnapshotQuery("Directive on both fragment(include true) and fragment selection(include false)", `query x {
		items {
			id
			...X @include(if: true)
		}
	}
	fragment X on Item {
		name @include(if: false)
		number
	}`)

	snap.SnapshotQuery("Directive on both fragment(include false) and fragment selection(include true)", `query x {
		items {
			id
			...X @include(if: false)
		}
	}
	fragment X on Item {
		name @include(if: true)
		number
	}`)

}

func TestDirectivesWithVariables(t *testing.T) {
	builtSchema := buildSchema()

	q := graphql.MustParse(`
		{
			...X @skip(if: $something)
		}
		fragment X on Query {
			items {
				name
			}
		}
	`, map[string]interface{}{"something": true})

	if err := graphql.PrepareQuery(context.Background(), builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := testgraphql.NewExecutorWrapper(t)

	val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{}, val)

	q = graphql.MustParse(`
		{
			...X @skip(if: $something)
		}
		fragment X on Query {
			items {
				name
			}
		}
	`, map[string]interface{}{"something": false})

	if err := graphql.PrepareQuery(context.Background(), builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"__key": int64(1),
				"name":  "\x01",
			},
			map[string]interface{}{
				"__key": int64(2),
				"name":  "\x02",
			},
			map[string]interface{}{
				"__key": int64(3),
				"name":  "\x03",
			},
			map[string]interface{}{
				"__key": int64(4),
				"name":  "\x04",
			},
			map[string]interface{}{
				"__key": int64(5),
				"name":  "\x05",
			},
		},
	}, val)
}

func TestDirectivesWithErrors(t *testing.T) {
	builtSchema := buildSchema()
	e := testgraphql.NewExecutorWrapper(t)

	q := graphql.MustParse(`
		{
			...X @skip(notif: $something)
		}
		fragment X on Query {
			items {
				name
			}
		}
	`, map[string]interface{}{"something": false})
	_, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "required argument in directive not provided: if")

	q = graphql.MustParse(`
	{
		...X @skip(if: $something)
	}
	fragment X on Query {
		items {
			name
		}
	}
`, map[string]interface{}{"something": "wrong type"})
	_, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "expected type boolean, found type string in \"if\" argument")

}

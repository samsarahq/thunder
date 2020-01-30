package graphql_test

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal/testgraphql"
)



func TestSkipDirectives(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	item := schema.Object("item", Item{})
	item.Key("id")
	item.FieldFunc("name", func(ctx context.Context, item Item)(string, error) {
		return string(item.Id), nil
	})
	query.FieldFunc("items", func(ctx context.Context) ([]Item,  error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList,nil
	})
	builtSchema := schema.MustBuild()


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
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	item := schema.Object("item", Item{})
	item.Key("id")
	item.FieldFunc("name", func(ctx context.Context, item Item)(string, error) {
		return string(item.Id), nil
	})
	query.FieldFunc("items", func(ctx context.Context) ([]Item,  error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList,nil
	})
	builtSchema := schema.MustBuild()


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
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	item := schema.Object("item", Item{})
	item.Key("id")
	item.FieldFunc("name", func(ctx context.Context, item Item)(string, error) {
		return string(item.Id), nil
	})
	query.FieldFunc("items", func(ctx context.Context) ([]Item,  error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList,nil
	})
	builtSchema := schema.MustBuild()


	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Directive with fragment on top level", `query x {
		...X @skip(if: true)
	}
	fragment X on Query {
		items {
			name
		}
	}`)

	snap.SnapshotQuery("Directive with fragment nested", `query x {
		items {
			id
			...X @skip(if: true)
		}
	}
	fragment X on Item {
		name
	}`)


}

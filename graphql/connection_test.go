package graphql_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/samsarahq/thunder/internal/testgraphql"
	"github.com/samsarahq/thunder/reactive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type User struct {
	Name     string
	Age      int
	resource *reactive.Resource
}

type Slow struct {
}

type Args struct {
	Additional string
}

type ManualArgs struct {
	Additional     string
	PaginationArgs schemabuilder.PaginationArgs
}

type Item struct {
	Id         int64
	FilterText string
	Number     int64
	String     string
	Float      float64
}

func TestConnection(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")
	inner.FieldFunc("innerConnection", func(args Args) []Item {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}
	}, schemabuilder.Paginated)
	inner.FieldFunc("innerConnectionWithFilter", func() []Item {
		return []Item{
			{Id: 1, FilterText: "can"},
			{Id: 2, FilterText: "man"},
			{Id: 3, FilterText: "cannot"},
			{Id: 4, FilterText: "soban"},
			{Id: 5, FilterText: "socan"},
		}

	}, schemabuilder.Paginated,
		schemabuilder.BatchFilterField("foo",
			func(ctx context.Context, i map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(i))
				for i, item := range i {
					myMap[i] = item.FilterText
				}
				return myMap, nil
			},
		),
		schemabuilder.BatchFilterField("bar",
			func(ctx context.Context, i map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(i))
				for i := range i {
					myMap[i] = ""
				}
				return myMap, nil
			},
		),
	)

	inner.FieldFunc("innerConnectionWithSort", func() []Item {
		return []Item{
			{Id: 1, Number: 1, String: "1", Float: 1.0},
			{Id: 2, Number: 3, String: "3", Float: 3.0},
			{Id: 3, Number: 5, String: "5", Float: 5.0},
			{Id: 4, Number: 2, String: "2", Float: 2.0},
			{Id: 5, Number: 4, String: "4", Float: 4.0},
		}
	},
		schemabuilder.Paginated,
		schemabuilder.BatchSortField(
			"numbers", func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			}),
		schemabuilder.SortField(
			"strings", func(ctx context.Context, i Item) string {
				return i.String
			}),
		schemabuilder.SortField(
			"floats", func(ctx context.Context, i Item) float64 {
				return i.Float
			}))
	inner.FieldFunc("innerConnectionNilArg", func() []Item {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}
	}, schemabuilder.Paginated)
	inner.FieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
	}, schemabuilder.Paginated)
	inner.FieldFunc("innerConnectionWithError", func(ctx context.Context, args Args) ([]*Item, error) {
		return nil, graphql.NewSafeError("this is an error")
	}, schemabuilder.Paginated)
	builtSchema := schema.MustBuild()

	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Pagination, first + after", `{
		inner {
			innerConnection(first: 1, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, last + before", `{
		inner {
			innerConnection(last: 2, before: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, no args given", `{
		inner {
			innerConnection(additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, nil args", `{
		inner {
			innerConnectionNilArg(first: 1, after: "") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, with ctx and error", `{
		inner {
			innerConnectionWithCtxAndError(first: 1, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, with ctx and error", `{
		inner {
			innerConnectionWithError(first: 1, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("Pagination, with error", `{
		inner {
			innerConnectionWithError(first: 1, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("Pagination, with error", `{
		inner {
			innerConnection(last: -2, before: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`, testgraphql.RecordError)

	snap.SnapshotQuery("Pagination, filter", `{
		inner {
			filterByCan: innerConnectionWithFilter(filterText: "can", filterTextFields: ["foo","wug"], first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						filterText
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			filterByBan: innerConnectionWithFilter(filterText: "ban", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						filterText
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, sorts", `{
		inner {
			numbersAsc: innerConnectionWithSort(sortBy: "numbers", sortOrder: "asc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						number
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			numbersDesc: innerConnectionWithSort(sortBy: "numbers", sortOrder: "desc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						number
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			stringsAsc: innerConnectionWithSort(sortBy: "strings", sortOrder: "asc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						string
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			stringsDesc: innerConnectionWithSort(sortBy: "strings", sortOrder: "desc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						string
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			floatsAsc: innerConnectionWithSort(sortBy: "floats", sortOrder: "asc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						float
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
			floatsDesc: innerConnectionWithSort(sortBy: "floats", sortOrder: "desc", first: 5, after: "") {
				totalCount
				edges {
					node {
						id
						float
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)
}

func TestPaginateBuildFailure(t *testing.T) {
	type Inner struct{}

	t.Run("slice type return error", func(t *testing.T) {
		schema := schemabuilder.NewSchema()

		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		item := schema.Object("item", Item{})
		item.Key("id")

		inner.FieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) (*Item, error) {
			return nil, nil
		}, schemabuilder.Paginated)
		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "paginated field func must return a slice type")
	})

	t.Run("key field error", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		_ = schema.Object("item", Item{})

		inner.FieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
			return nil, nil
		}, schemabuilder.Paginated)
		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "a key field must be registered for paginated objects")
	})

	t.Run("key field doesn't exist error", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("wrongField")
		inner.FieldFunc("innerConnectionWithWrongKey", func(ctx context.Context, args Args) ([]StructWithKey, error) {
			return nil, nil
		}, schemabuilder.Paginated)
		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "key field doesn't exist on object")
	})

	t.Run("empty filterText return", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("id")
		inner.FieldFunc("connection", func(ctx context.Context, args Args) ([]StructWithKey, error) {
			return nil, nil
		},
			schemabuilder.Paginated,
			schemabuilder.BatchFilterField(
				"noargs",
				func() {},
			))

		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported return type <nil>")
	})

	t.Run("non-string filterText return", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("id")
		inner.FieldFunc("connection", func(ctx context.Context, args Args) ([]StructWithKey, error) {
			return nil, nil
		}, schemabuilder.Paginated, schemabuilder.BatchFilterField(
			"intReturn", func() int64 { return 0 },
		))

		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported return type int64")
	})

	t.Run("filterText with args", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner { return Inner{} })

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("id")
		inner.FieldFunc("connection", func(ctx context.Context, i *Inner, args Args) ([]StructWithKey, error) {
			return nil, nil
		}, schemabuilder.Paginated, schemabuilder.BatchFilterField(
			"someArgs", func(ctx context.Context, i map[batch.Index]*StructWithKey, args Args) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(i))
				for i := range i {
					myMap[i] = ""
				}
				return myMap, nil
			},
		))

		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "text filter fields can't take arguments")
	})

	t.Run("non-string sort return", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner {
			return Inner{}
		})

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("id")
		inner.FieldFunc("connection", func(ctx context.Context, args Args) ([]StructWithKey, error) {
			return nil, nil
		},
			schemabuilder.Paginated,
			schemabuilder.SortField(
				"badReturn",
				func() struct{} { return struct{}{} },
			))

		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported return type struct {}")
	})

	t.Run("sort with args", func(t *testing.T) {
		schema := schemabuilder.NewSchema()
		type StructWithKey struct {
			Id int64
		}
		query := schema.Query()
		query.FieldFunc("inner", func() Inner { return Inner{} })

		inner := schema.Object("inner", Inner{})
		object := schema.Object("structWithKey", StructWithKey{})
		object.Key("id")
		inner.FieldFunc("connection", func(ctx context.Context, i *Inner, args Args) ([]StructWithKey, error) {
			return nil, nil
		}, schemabuilder.Paginated, schemabuilder.SortField(
			"someArgs", func(ctx context.Context, i *StructWithKey, args Args) string {
				return ""
			},
		))

		_, err := schema.Build()
		require.Error(t, err)
		require.Contains(t, err.Error(), "sort fields can't take arguments")
	})
}

func TestPaginateNodeTypeFailure(t *testing.T) {
	schema := schemabuilder.NewSchema()
	query := schema.Query()

	type Inner struct {
	}

	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})

	inner.PaginateFieldFunc("innerConnectionWithScalar", func(ctx context.Context, args Args) ([]Item, error) {
		return nil, nil
	})

	badMethodStr := "bad method inner on type schemabuilder.query:"
	_, err := schema.Build()
	if err == nil || err.Error() != fmt.Sprintf("%s graphql_test.Item must be a struct and registered as an object along with its key", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

	schema = schemabuilder.NewSchema()
	query = schema.Query()

	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner = schema.Object("inner", Inner{})

	inner.PaginateFieldFunc("innerConnectionWithScalar", func(ctx context.Context, args Args) ([]string, error) {
		return nil, nil
	})

	badMethodStr = "bad method inner on type schemabuilder.query:"
	_, err = schema.Build()
	if err == nil || err.Error() != fmt.Sprintf("%s string must be a struct and registered as an object along with its key", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

}

type EmbeddedArgs struct {
	schemabuilder.PaginationArgs
	Additional string
}

func TestEmbeddedArgs(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")
	inner.PaginateFieldFunc("innerConnection", func(args EmbeddedArgs) ([]Item, schemabuilder.PaginationInfo, error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList,
			schemabuilder.PaginationInfo{
				HasNextPage:    true,
				HasPrevPage:    false,
				TotalCountFunc: func() int64 { return int64(5) },
			}, nil
	})
	builtSchema := schema.MustBuild()
	q := graphql.MustParse(`
		{
			inner {
				innerConnection(first: 5, after: "", additional: "jk") {
					totalCount
					edges {
						node {
							id
						}
						cursor
					}
					pageInfo {
						hasNextPage
						hasPrevPage
						startCursor
						endCursor
					}
				}
			}
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := testgraphql.NewExecutorWrapper(t)
	val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)

	assert.Equal(t, map[string]interface{}{
		"inner": map[string]interface{}{
			"innerConnection": map[string]interface{}{
				"totalCount": float64(5),
				"edges": []interface{}{
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(1),
							"id":    float64(1),
						},
						"cursor": "MQ==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(2),
							"id":    float64(2),
						},
						"cursor": "Mg==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(3),
							"id":    float64(3),
						},
						"cursor": "Mw==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(4),
							"id":    float64(4),
						},
						"cursor": "NA==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(5),
							"id":    float64(5),
						},
						"cursor": "NQ==",
					},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": true,
					"hasPrevPage": false,
					"startCursor": "MQ==",
					"endCursor":   "NQ==",
				},
			},
		},
	}, internal.AsJSON(val))

	schema = schemabuilder.NewSchema()

	query = schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner = schema.Object("inner", Inner{})
	item = schema.Object("item", Item{})
	item.Key("id")
	inner.PaginateFieldFunc("innerConnection", func(args struct {
		schemabuilder.PaginationArgs
		First string
	}) []Item {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList
	})
	_, err = schema.Build()

	badMethodStr := "bad method inner on type schemabuilder.query:"
	if err == nil || err.Error() != fmt.Sprintf("%v these arg names are restricted: First, After, Last and Before", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

}

func TestEmbeddedFail(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")
	inner.PaginateFieldFunc("innerConnection", func(args EmbeddedArgs) ([]Item, schemabuilder.PaginationInfo, error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList,
			schemabuilder.PaginationInfo{
				HasNextPage:    true,
				HasPrevPage:    false,
				TotalCountFunc: func() int64 { return int64(5) },
			}, nil
	})

	badMethodStr := "bad method inner on type schemabuilder.query:"
	_, err := schema.Build()
	if err != nil && err.Error() != fmt.Sprintf("%s if pagination args are embedded then pagination info must be included as a return value", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}
}

func TestPaginatedFilters(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")
	inner.FieldFunc("innerConnection", func(args Args) []Item {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}
	}, schemabuilder.Paginated)
	inner.FieldFunc("innerConnectionWithFilter", func() []Item {
		return []Item{
			{Id: 1, FilterText: "can", String: "a"},
			{Id: 2, FilterText: "man", String: "a"},
			{Id: 3, FilterText: "cannot", String: "a"},
			{Id: 4, FilterText: "soban", String: "a"},
			{Id: 5, FilterText: "socan", String: "a"},
			{Id: 6, FilterText: "aan", String: "a"},
			{Id: 7, FilterText: "jan", String: "a"},
			{Id: 8, FilterText: "ban", String: "a"},
			{Id: 9, FilterText: "dan", String: "a"},
			{Id: 10, FilterText: "ean", String: "a"},
			{Id: 11, FilterText: "fan", String: "a"},
			{Id: 12, FilterText: "gan", String: "a"},
		}

	}, schemabuilder.Paginated,
		schemabuilder.BatchFilterField("filterTextBatched",
			func(items map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(items))
				for i, item := range items {
					myMap[i] = item.FilterText
				}
				return myMap, nil
			},
		),
		schemabuilder.FilterField("filterTextNotBatched",
			func(item Item) string {
				return item.FilterText
			},
		),
		schemabuilder.BatchFilterFieldWithFallback("filterTextBatchWithFallbackTrue",
			func(items map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(items))
				for i, item := range items {
					myMap[i] = item.FilterText
				}
				return myMap, nil
			},
			func(item Item) (string, error) {
				return item.FilterText, nil
			},
			func(context.Context) bool {
				return true
			}),
		schemabuilder.BatchFilterFieldWithFallback("filterTextBatchWithFallbackFalse",
			func(items map[batch.Index]Item) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(items))
				for i, item := range items {
					myMap[i] = item.FilterText
				}
				return myMap, nil
			},
			func(item Item) (string, error) {
				return item.FilterText, nil
			},
			func(context.Context) bool {
				return false
			}),
	)
	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			inner {
				innerConnectionWithFilter(filterText: "can", first: 4, after: "") {
					totalCount
					edges {
						node {
							id
						}
						cursor
					}
				}
			}
		}`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := testgraphql.NewExecutorWrapper(t)
	val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"inner": map[string]interface{}{
			"innerConnectionWithFilter": map[string]interface{}{
				"totalCount": float64(3),
				"edges": []interface{}{
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(1),
							"id":    float64(1),
						},
						"cursor": "MQ==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(3),
							"id":    float64(3),
						},
						"cursor": "Mw==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": float64(5),
							"id":    float64(5),
						},
						"cursor": "NQ==",
					},
				},
			},
		},
	}, internal.AsJSON(val))
}

func TestPaginatedSorts(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")
	inner.FieldFunc("innerConnection", func(args Args) []Item {
		return []Item{{Id: 2}, {Id: 5}, {Id: 4}, {Id: 3}, {Id: 1}}
	}, schemabuilder.Paginated)
	inner.FieldFunc("innerConnectionWithSort", func() []Item {
		return []Item{
			{Id: 1, Number: 1, String: "1", Float: 1.0},
			{Id: 2, Number: 3, String: "3", Float: 3.0},
			{Id: 3, Number: 5, String: "5", Float: 5.0},
			{Id: 4, Number: 2, String: "2", Float: 2.0},
			{Id: 5, Number: 4, String: "4", Float: 4.0},
		}
	},
		schemabuilder.Paginated,
		schemabuilder.BatchSortField(
			"numbersBatched", func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			}),
		schemabuilder.SortField(
			"numbers", func(ctx context.Context, item Item) int64 {
				return item.Number
			}),
		schemabuilder.BatchSortFieldWithFallback("numbersBatchedWithFallbackFalse",
			func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			},
			func(ctx context.Context, item Item) (int64, error) {
				return item.Number, nil
			},
			func(context.Context) bool {
				return false
			},
		),
		schemabuilder.BatchSortFieldWithFallback("numbersBatchedWithFallbackTrue",
			func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			},
			func(ctx context.Context, item Item) (int64, error) {
				return item.Number, nil
			},
			func(context.Context) bool {
				return true
			},
		),
		schemabuilder.BatchSortFieldWithFallback("numbersIncorrectFallback",
			func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, item := range items {
					myMap[i] = item.Number
				}
				return myMap, nil
			},
			func(ctx context.Context, item Item) (int64, error) {
				return 0, nil
			},
			func(context.Context) bool {
				return true
			},
		),
		schemabuilder.BatchSortFieldWithFallback("numbersIncorrectBatch",
			func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]int64, error) {
				myMap := make(map[batch.Index]int64, len(items))
				for i, _ := range items {
					myMap[i] = 0
				}
				return myMap, nil
			},
			func(ctx context.Context, item Item) (int64, error) {
				return item.Number, nil
			},
			func(context.Context) bool {
				return false
			},
		),
	)

	builtSchema := schema.MustBuild()
	// Test querries that succesfully sort
	queries := []*graphql.Query{
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbersBatchedWithFallbackTrue", sortOrder: "asc", first: 4, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbersBatchedWithFallbackFalse", sortOrder: "asc", first: 4, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbersBatched", sortOrder: "asc", first: 4, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbers", sortOrder: "asc", first: 4, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
	}

	for _, q := range queries {
		if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
			t.Error(err)
		}
		e := testgraphql.NewExecutorWrapper(t)
		val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
		assert.Nil(t, err)
		assert.Equal(t, map[string]interface{}{
			"inner": map[string]interface{}{
				"innerConnectionWithSort": map[string]interface{}{
					"totalCount": float64(5),
					"edges": []interface{}{
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(1),
								"id":     float64(1),
								"number": float64(1),
							},
						},
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(4),
								"id":     float64(4),
								"number": float64(2),
							},
						},
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(2),
								"id":     float64(2),
								"number": float64(3),
							},
						},
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(5),
								"id":     float64(5),
								"number": float64(4),
							},
						},
					},
				},
			},
		}, internal.AsJSON(val))
	}

	// Test querries that don't succesfully sort
	queries = []*graphql.Query{
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbersIncorrectFallback", sortOrder: "asc", last: 3, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
		graphql.MustParse(`
		{
			inner {
				innerConnectionWithSort(sortBy: "numbersIncorrectBatch", sortOrder: "asc", last: 3, after: "") {
					totalCount
					edges {
						node {
							id
							number
						}
					}
				}
			}
		}`, nil),
	}

	for _, q := range queries {
		if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
			t.Error(err)
		}
		e := testgraphql.NewExecutorWrapper(t)
		val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
		assert.Nil(t, err)
		assert.Equal(t, map[string]interface{}{
			"inner": map[string]interface{}{
				"innerConnectionWithSort": map[string]interface{}{
					"totalCount": float64(5),
					"edges": []interface{}{
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(2),
								"id":     float64(2),
								"number": float64(3),
							},
						},
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(5),
								"id":     float64(5),
								"number": float64(4),
							},
						},
						map[string]interface{}{
							"node": map[string]interface{}{
								"__key":  float64(3),
								"id":     float64(3),
								"number": float64(5),
							},
						},
					},
				},
			},
		}, internal.AsJSON(val))
	}
}

func TestConnectionManual(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")

	inner.FieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
	}, schemabuilder.Paginated)

	inner.FieldFunc("innerConnectionWithCtxAndErrorManual", func(ctx context.Context, args ManualArgs) ([]Item, schemabuilder.PaginationInfo, error) {
		var info schemabuilder.PaginationInfo
		info.TotalCountFunc = func() int64 { return 5 }
		info.HasNextPage = true
		info.HasPrevPage = false
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}}, info, nil
	}, schemabuilder.Paginated)

	shouldUseFallback := true
	inner.ManualPaginationWithFallback("innerConnectionWithCtxAndErrorManualAndFallback",
		func(ctx context.Context, args ManualArgs) ([]Item, schemabuilder.PaginationInfo, error) {
			var info schemabuilder.PaginationInfo
			info.TotalCountFunc = func() int64 { return 5 }
			info.HasNextPage = true
			info.HasPrevPage = false
			info.Pages = []string{}
			return []Item{{Id: 1}, {Id: 2}, {Id: 3}}, info, nil
		},
		func(ctx context.Context, args Args) ([]Item, error) {
			return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
		},
		func(ctx context.Context) bool {
			return shouldUseFallback
		},
		schemabuilder.Paginated)

	inner.ManualPaginationWithFallback("manualPaginationAndFallbackWithFiltersAndSorts",
		func(ctx context.Context, args ManualArgs) ([]Item, schemabuilder.PaginationInfo, error) {
			var info schemabuilder.PaginationInfo
			info.TotalCountFunc = func() int64 { return 5 }
			info.HasNextPage = true
			info.HasPrevPage = false
			info.Pages = []string{}
			return []Item{{Id: 1}, {Id: 2}, {Id: 3}}, info, nil
		},
		func(ctx context.Context, args Args) ([]Item, error) {
			return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
		},
		func(ctx context.Context) bool {
			return shouldUseFallback
		},
		schemabuilder.Paginated,
		schemabuilder.FilterField("number", func(i Item) string {
			return strconv.FormatInt(i.Id, 10)
		}),
		schemabuilder.SortField("number", func(i Item) string { return string(i.Id) }),
	)

	builtSchema := schema.MustBuild()

	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Pagination, with ctx and error", `{
		inner {
			innerConnectionWithCtxAndError(first: 3, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, with ctx and error manual", `{
		inner {
			innerConnectionWithCtxAndErrorManual(first: 3, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	shouldUseFallback = true
	snap.SnapshotQuery("Pagination, uses manual pagination instead of fallback", `{
		inner {
			innerConnectionWithCtxAndErrorManualAndFallback(first: 3, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, uses manual pagination instead of fallback, with filtertext", `{
		inner {
			manualPaginationAndFallbackWithFiltersAndSorts(first: 3, after: "", filterText: "1", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination, uses manual pagination instead of fallback, with sort by desc", `{
		inner {
			manualPaginationAndFallbackWithFiltersAndSorts(first: 3, after: "", sortBy: "number", sortOrder:"desc", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

	shouldUseFallback = false
	snap.SnapshotQuery("Pagination, uses fallback method", `{
		inner {
			innerConnectionWithCtxAndErrorManualAndFallback(first: 3, after: "", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
					pages
				}
			}
		}
	}`)

}

func TestConnectionCursor(t *testing.T) {
	schema := schemabuilder.NewSchema()
	type Inner struct {
	}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner := schema.Object("inner", Inner{})
	item := schema.Object("item", Item{})
	item.Key("id")

	inner.FieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
		return []Item{{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4}, {Id: 5}}, nil
	}, schemabuilder.Paginated)

	builtSchema := schema.MustBuild()

	snap := testgraphql.NewSnapshotter(t, builtSchema)
	defer snap.Verify()

	snap.SnapshotQuery("Pagination first and after", `{
		inner {
			innerConnectionWithCtxAndError(first: 2, after: "Mw==", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination first and after that doesnt match anything", `{
		inner {
			innerConnectionWithCtxAndError(first: 2, after: "BAD", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination only first", `{
		inner {
			innerConnectionWithCtxAndError(first: 2,  additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination last and before", `{
		inner {
			innerConnectionWithCtxAndError(last: 2, before: "Mw==", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination last and before that doens't match anything", `{
		inner {
			innerConnectionWithCtxAndError(last: 2, before: "BAD", additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

	snap.SnapshotQuery("Pagination only last", `{
		inner {
			innerConnectionWithCtxAndError(last: 2, additional: "jk") {
				totalCount
				edges {
					node {
						id
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPrevPage
					startCursor
					endCursor
				}
			}
		}
	}`)

}

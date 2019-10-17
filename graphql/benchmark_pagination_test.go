package graphql_test

import (
	"context"
	"testing"

	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/stretchr/testify/require"
)

func pagintedQueryWithFilterBenchmark(b *testing.B, n int, batchFunc bool, expensive bool) {
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
	filterTexts := [5]string{"can", "man", "cannot", "soban", "socan"}
	items := make([]Item, n, n)
	for i := 0; i < n; i++ {
		items[i] = Item{Id: int64(i), FilterText: filterTexts[i%5], String: "a"}
	}
	if !batchFunc {
		if expensive {
			inner.FieldFunc("innerConnectionWithFilter", func() []Item {
				return items
			}, schemabuilder.Paginated,
				schemabuilder.FilterField("foo",
					func(ctx context.Context, i Item) string {
						return i.FilterText
					},
				),
				schemabuilder.FilterField("bar",
					func(ctx context.Context, i Item) string {
						return i.String
					},
				),
			)
		} else {
			inner.FieldFunc("innerConnectionWithFilter", func() []Item {
				return items
			}, schemabuilder.Paginated,
				schemabuilder.FilterField("foo",
					func(i Item) string {
						return i.FilterText
					},
				),
				schemabuilder.FilterField("bar",
					func(i Item) string {
						return i.String
					},
				),
			)
		}

	} else {
		inner.FieldFunc("innerConnectionWithFilter", func() []Item {
			return items
		}, schemabuilder.Paginated,
			schemabuilder.BatchFilterField("foo",
				func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]string, error) {
					myMap := make(map[batch.Index]string, len(items))
					for i, item := range items {
						myMap[i] = item.FilterText
					}
					return myMap, nil
				},
			),
			schemabuilder.BatchFilterField("bar",
				func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]string, error) {
					myMap := make(map[batch.Index]string, len(items))
					for i, item := range items {
						myMap[i] = item.String
					}
					return myMap, nil
				},
			),
		)
	}
	builtSchema := schema.MustBuild()
	q := graphql.MustParse(`
		{
			inner {
				innerConnectionWithFilter(filterText: "can",first: 4, after: "") {
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
	exeuctor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := exeuctor.Execute(context.Background(), builtSchema.Query, nil, q)
		require.NoError(b, err)
	}

}

func BenchmarkFiltersBatched10Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 10, true, false)
}

func BenchmarkFiltersNotBatched10Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 10, false, false)
}
func BenchmarkFiltersNotBatched10ItemsExpensive(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 10, false, true)
}

func BenchmarkFiltersBatched100Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 100, true, false)
}

func BenchmarkFiltersNotBatched100Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 100, false, false)
}

func BenchmarkFiltersNotBatched100ItemsExpensive(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 100, false, true)
}
func BenchmarkFiltersBatched1000Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 1000, true, false)
}

func BenchmarkFiltersNotBatched1000Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 1000, false, false)
}

func BenchmarkFiltersNotBatched1000ItemsExpensive(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 1000, false, true)
}

func pagintedQueryWithSortBenchmark(b *testing.B, n int, batchFunc bool, expensive bool) {
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
	filterTexts := [5]string{"can", "man", "cannot", "soban", "socan"}
	items := make([]Item, n, n)
	for i := 0; i < n; i++ {
		items[i] = Item{Id: int64(i), FilterText: filterTexts[i%5], String: "a"}
	}
	if !batchFunc {
		if expensive {
			inner.FieldFunc("innerConnectionWithFilter", func() []Item {
				return items
			}, schemabuilder.Paginated,
				schemabuilder.SortField("foo",
					func(ctx context.Context, i Item) string {
						return i.FilterText
					},
				),
				schemabuilder.SortField("bar",
					func(ctx context.Context, i Item) string {
						return i.String
					},
				),
			)
		} else {
			inner.FieldFunc("innerConnectionWithFilter", func() []Item {
				return items
			}, schemabuilder.Paginated,
				schemabuilder.SortField("foo",
					func(i Item) string {
						return i.FilterText
					},
				),
				schemabuilder.SortField("bar",
					func(i Item) string {
						return i.String
					},
				),
			)
		}

	} else {
		inner.FieldFunc("innerConnectionWithFilter", func() []Item {
			return items
		}, schemabuilder.Paginated,
			schemabuilder.BatchSortField("foo",
				func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]string, error) {
					myMap := make(map[batch.Index]string, len(items))
					for i, item := range items {
						myMap[i] = item.FilterText
					}
					return myMap, nil
				},
			),
			schemabuilder.BatchSortField("bar",
				func(ctx context.Context, items map[batch.Index]Item) (map[batch.Index]string, error) {
					myMap := make(map[batch.Index]string, len(items))
					for i, item := range items {
						myMap[i] = item.String
					}
					return myMap, nil
				},
			),
		)
	}
	builtSchema := schema.MustBuild()
	q := graphql.MustParse(`
		{
			inner {
				innerConnectionWithFilter(sortBy: "foo",first: 4, after: "") {
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
	exeuctor := graphql.NewExecutor(graphql.NewImmediateGoroutineScheduler())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := exeuctor.Execute(context.Background(), builtSchema.Query, nil, q)
		require.NoError(b, err)
	}

}

func BenchmarkSortsBatched10Items(b *testing.B) {
	pagintedQueryWithFilterBenchmark(b, 10, true, false)
}

func BenchmarkSortsNotBatched10Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 10, false, false)
}

func BenchmarkSortsNotBatchedExpensive10Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 10, false, true)
}

func BenchmarkSortsBatched100Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 100, true, false)
}

func BenchmarkSortsNotBatched100Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 100, false, false)
}

func BenchmarkSortsNotBatchedExpensive100Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 100, false, true)
}

func BenchmarkSortsBatched1000Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 1000, true, false)
}

func BenchmarkSortsNotBatched1000Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 1000, false, false)
}

func BenchmarkSortsNotBatchedExpensive1000Items(b *testing.B) {
	pagintedQueryWithSortBenchmark(b, 1000, false, true)
}

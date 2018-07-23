package graphql_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/samsarahq/thunder/concurrencylimiter"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/samsarahq/thunder/reactive"
	"github.com/stretchr/testify/assert"
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

type Item struct {
	Id int64
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
	inner.PaginateFieldFunc("innerConnection", func(args Args) []Item {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList
	})
	inner.PaginateFieldFunc("innerConnectionNilArg", func() []Item {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList
	})
	inner.PaginateFieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
		retList := make([]Item, 5)
		retList[0] = Item{Id: 1}
		retList[1] = Item{Id: 2}
		retList[2] = Item{Id: 3}
		retList[3] = Item{Id: 4}
		retList[4] = Item{Id: 5}
		return retList, nil
	})
	inner.PaginateFieldFunc("innerConnectionWithError", func(ctx context.Context, args Args) ([]*Item, error) {
		return nil, graphql.NewSafeError("this is an error")
	})
	builtSchema := schema.MustBuild()

	// Test for the normal case with first and after.
	q := graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := graphql.Executor{}
	val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)

	assert.Equal(t, map[string]interface{}{
		"inner": map[string]interface{}{
			"innerConnection": map[string]interface{}{
				"totalCount": int64(5),
				"edges": []interface{}{map[string]interface{}{
					"node": map[string]interface{}{
						"__key": int64(1),
						"id":    int64(1),
					},
					"cursor": "MQ==",
				},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": true,
					"hasPrevPage": false,
					"startCursor": "MQ==",
					"endCursor":   "MQ==",
				},
			},
		},
	}, val)

	// Test for last and before with pages.
	q = graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"inner": map[string]interface{}{
			"innerConnection": map[string]interface{}{
				"totalCount": int64(5),
				"edges": []interface{}{
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": int64(4),
							"id":    int64(4),
						},
						"cursor": "NA==",
					},
					map[string]interface{}{
						"node": map[string]interface{}{
							"__key": int64(5),
							"id":    int64(5),
						},
						"cursor": "NQ==",
					},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"hasPrevPage": true,
					"startCursor": "NA==",
					"endCursor":   "NQ==",
					"pages":       []interface{}{"MQ==", "Mw==", "NQ=="},
				},
			},
		},
	}, val)

	q = graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)

	q = graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"inner": map[string]interface{}{
			"innerConnectionWithCtxAndError": map[string]interface{}{
				"totalCount": int64(5),
				"edges": []interface{}{map[string]interface{}{
					"node": map[string]interface{}{
						"__key": int64(1),
						"id":    int64(1),
					},
					"cursor": "MQ==",
				},
				},
				"pageInfo": map[string]interface{}{
					"hasNextPage": true,
					"hasPrevPage": false,
					"startCursor": "MQ==",
					"endCursor":   "MQ==",
				},
			},
		},
	}, val)

	q = graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	_, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "this is an error" {
		t.Errorf("bad error: %v", err)
	}

	q = graphql.MustParse(`
		{
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
	    }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "last should be a non-negative integer" {
		t.Errorf("bad error: %v", err)
	}

}

func TestPaginateBuildFailure(t *testing.T) {
	badMethodStr := "bad method inner on type schemabuilder.query:"

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

	inner.PaginateFieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) (*Item, error) {
		return nil, nil
	})
	_, err := schema.Build()
	if err == nil || err.Error() != fmt.Sprintf("%v paginated field func must return a slice type", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

	schema = schemabuilder.NewSchema()
	query = schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner = schema.Object("inner", Inner{})
	item = schema.Object("item", Item{})

	inner.PaginateFieldFunc("innerConnectionWithCtxAndError", func(ctx context.Context, args Args) ([]Item, error) {
		return nil, nil
	})
	_, err = schema.Build()
	if err == nil || err.Error() != fmt.Sprintf("%v a key field must be registered for paginated objects", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

	schema = schemabuilder.NewSchema()
	type StructWithKey struct {
		Id int64
	}
	query = schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	inner = schema.Object("inner", Inner{})
	object := schema.Object("structWithKey", StructWithKey{})
	object.Key("wrongField")
	inner.PaginateFieldFunc("innerConnectionWithWrongKey", func(ctx context.Context, args Args) ([]StructWithKey, error) {
		return nil, nil
	})
	_, err = schema.Build()
	if err == nil || err.Error() != fmt.Sprintf("%v key field doesn't exist on object", badMethodStr) {
		t.Errorf("bad error: %v", err)
	}

	schema = schemabuilder.NewSchema()
	query = schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
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

func TestPathError(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type Inner struct{}

	query := schema.Query()
	query.FieldFunc("inner", func() Inner {
		return Inner{}
	})

	query.FieldFunc("safe", func() error {
		return graphql.NewSafeError("safe safe")
	})

	_ = schema.Mutation()

	type Expensive struct{}

	inner := schema.Object("inner", Inner{})
	inner.FieldFunc("expensive", func(ctx context.Context) Expensive {
		return Expensive{}
	})
	inner.FieldFunc("inners", func(ctx context.Context) []Inner {
		return []Inner{Inner{}}
	})

	nested := schema.Object("expensive", Expensive{})
	nested.FieldFunc("expensives", func(ctx context.Context) []Expensive {
		return []Expensive{Expensive{}}
	})

	nested.FieldFunc("err", func() error {
		return errors.New("no good, bad")
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			inner { inners { expensive { expensives { err } } } }
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	_, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "inner.inners.0.expensive.expensives.0.err: no good, bad" {
		t.Errorf("bad error: %v", err)
	}

	q = graphql.MustParse(`
		{
			safe
		}`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	_, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	if err == nil || err.Error() != "safe safe" {
		t.Errorf("bad error: %v", err)
	}
	if _, ok := err.(graphql.SanitizedError); !ok {
		t.Errorf("safe not safe")
	}

}

func TestEnum(t *testing.T) {
	schema := schemabuilder.NewSchema()

	type enumType int32
	type enumType2 float64

	schema.Enum(enumType(1), map[string]interface{}{
		"firstField":  enumType(1),
		"secondField": enumType(2),
		"thirdField":  enumType(3),
	})
	schema.Enum(enumType2(1.2), map[string]float64{
		"this": float64(1.2),
		"is":   float64(3.2),
		"a":    float64(4.3),
		"map":  float64(5.3),
	})

	query := schema.Query()
	query.FieldFunc("inner", func(args struct {
		EnumField enumType
	}) enumType {
		return args.EnumField
	})
	query.FieldFunc("inner2", func(args struct {
		EnumField2 enumType2
	}) enumType2 {
		return args.EnumField2
	})

	query.FieldFunc("optional", func(args struct {
		EnumField *enumType
	}) enumType {
		if args.EnumField != nil {
			return *args.EnumField
		} else {
			return enumType(4)
		}
	})

	query.FieldFunc("pointerret", func(args struct {
		EnumField *enumType
	}) *enumType {
		return args.EnumField
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			inner(enumField: firstField)
		}
		`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	val, err := e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"inner": "firstField",
	}, val)

	q = graphql.MustParse(`
		{
			inner2(enumField2: this)
		}
		`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"inner2": "this",
	}, val)

	q = graphql.MustParse(`
		{
			inner(enumField: wrongField)
		}
		`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err == nil {
		t.Error(err)
	}

	q = graphql.MustParse(`
		{
			optional(enumField: firstField)
		}
		`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"optional": "firstField",
	}, val)

	q = graphql.MustParse(`
		{
			pointerret(enumField: firstField)
		}
		`, nil)
	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e = graphql.Executor{}
	val, err = e.Execute(context.Background(), builtSchema.Query, nil, q)
	assert.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"pointerret": enumType(1),
	}, val)

}

// TestEndToEndAwaitAndCache tests that slow fields get run in parallel and cached.
//
// The test verifies that the `slow` field on user, which sleeps for 100ms, gets
// run in parallel by verifying the total runtime over several users.
//
// The test verifies that a `count` sub-field of the `slow` field is cached by
// invalidating a single `slow` call, and tracking the number of calls to count.
func TestEndToEndAwaitAndCache(t *testing.T) {
	users := []*User{
		{Name: "Alice", Age: 5, resource: reactive.NewResource()},
		{Name: "Bob", Age: 6, resource: reactive.NewResource()},
		{Name: "Charlie", Age: 7, resource: reactive.NewResource()},
	}

	var mu sync.Mutex
	calls := 0

	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func(ctx context.Context) []*User {
		return users
	})

	_ = schema.Mutation()

	user := schema.Object("User", User{})
	user.FieldFunc("slow", func(ctx context.Context, u *User) *Slow {
		reactive.AddDependency(ctx, u.resource)
		time.Sleep(100 * time.Millisecond)
		return new(Slow)
	})

	slow := schema.Object("Slow", Slow{})
	slow.FieldFunc("count", func() bool {
		mu.Lock()
		calls++
		mu.Unlock()
		return true
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			users {
				name
				slow { count }
            }
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	results := make(chan interface{})

	start := time.Now()
	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		e := graphql.Executor{}
		result, err := e.Execute(ctx, builtSchema.Query, nil, q)
		if err != nil {
			t.Error(err)
		}

		results <- internal.AsJSON(result)
		return nil, nil
	}, 0)
	defer rerunner.Stop()

	result := <-results
	duration := time.Since(start)
	if duration > 150*time.Millisecond {
		t.Errorf("did not execute in parallel; duration %v > 150ms", duration)
	}
	if !reflect.DeepEqual(result, internal.ParseJSON(`
		{"users": [
			{"name": "Alice", "slow": {"count": true}},
			{"name": "Bob", "slow": {"count": true}},
			{"name": "Charlie", "slow": {"count": true}}
        ]}`)) {
		t.Error("bad value")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls to slow, got %d", calls)
	}

	start = time.Now()
	users[0].resource.Strobe()
	result = <-results
	duration = time.Since(start)
	if duration > 150*time.Millisecond {
		t.Errorf("did not execute in parallel; duration %v > 150ms", duration)
	}
	if !reflect.DeepEqual(result, internal.ParseJSON(`
		{"users": [
			{"name": "Alice", "slow": {"count": true}},
			{"name": "Bob", "slow": {"count": true}},
			{"name": "Charlie", "slow": {"count": true}}
        ]}`)) {
		t.Error("bad value")
	}
	if calls != 4 {
		t.Errorf("expected 4 total calls to slow, got %d", calls)
	}
}

func verifyArgumentOption(t *testing.T, query graphql.Type, queryString string, variables map[string]interface{}, expectedResult string) {
	q := graphql.MustParse(queryString, variables)

	if err := graphql.PrepareQuery(query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := graphql.Executor{}
	result, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(internal.AsJSON(result), internal.ParseJSON(expectedResult)) {
		t.Error(internal.AsJSON(result))
	}
}

// TestArgumentOptionality tests that optional arguments can be omitted from
// query variables and that mandatory arguments must be included.
func TestArgumentOptionality(t *testing.T) {
	schema := schemabuilder.NewSchema()
	query := schema.Query()

	query.FieldFunc("optional", func(args struct{ X *int64 }) int64 {
		if args.X != nil {
			return *args.X
		}
		return -1
	})

	query.FieldFunc("mandatory", func(args struct{ X int64 }) int64 {
		return args.X
	})

	_ = schema.Mutation()
	builtSchema := schema.MustBuild()
	emptyVariables := map[string]interface{}{}
	filledVariables := map[string]interface{}{
		"testArg": float64(5),
	}

	// An optional argument that is passed in returns successfully.
	verifyArgumentOption(t, builtSchema.Query, `
		query getOptional($testArg: int64) {
			optional(x: $testArg)
		}`, filledVariables, `{"optional": 5}`)

	// An optional argument that is omitted returns successfully.
	verifyArgumentOption(t, builtSchema.Query, `
			query getOptional($testArg: int64) {
				optional(x: $testArg)
			}`, emptyVariables, `{"optional": -1}`)

	// A mandatory argument that is passed in returns successfully.
	verifyArgumentOption(t, builtSchema.Query, `
		query getMandatory($testArg: int64!) {
			mandatory(x: $testArg)
		}`, filledVariables, `{"mandatory": 5}`)
}

// TestConcurrencyLimiterDeadlock tests that the executor does not cause a
// concurrency limit deadlock by holding on to tokens after a resolver finishes
// running.
func TestConcurrencyLimiterDeadlock(t *testing.T) {
	var mu sync.Mutex
	calls := 0

	schema := schemabuilder.NewSchema()

	query := schema.Query()
	query.FieldFunc("users", func(ctx context.Context) []*User {
		var users []*User
		for i := 0; i < 200; i++ {
			users = append(users, &User{})
		}
		return users
	})

	_ = schema.Mutation()

	user := schema.Object("User", User{})
	user.FieldFunc("slow", func(ctx context.Context, u *User) *Slow {
		time.Sleep(10 * time.Millisecond)
		return &Slow{}
	})

	slow := schema.Object("Slow", Slow{})
	slow.FieldFunc("count", func() bool {
		mu.Lock()
		calls++
		mu.Unlock()
		return true
	})

	builtSchema := schema.MustBuild()

	q := graphql.MustParse(`
		{
			users {
				one: slow { count }
				two: slow { count }
            }
        }`, nil)

	if err := graphql.PrepareQuery(builtSchema.Query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	rerunner := reactive.NewRerunner(context.Background(), func(ctx context.Context) (interface{}, error) {
		defer wg.Done()
		e := graphql.Executor{}
		ctx = concurrencylimiter.With(ctx, 100)

		_, err := e.Execute(ctx, builtSchema.Query, nil, q)
		if err != nil {
			t.Error(err)
		}

		assert.Equal(t, 2*200, calls)
		return nil, nil
	}, 0)

	wg.Wait()
	defer rerunner.Stop()
}

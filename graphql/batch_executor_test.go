package graphql_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/require"
)

func TestNonExpensiveExecution(t *testing.T) {
	type Object struct {
		Key string
		Num int
	}
	type Object2 struct {
		Key2 string
		Num2 int
	}
	type UnionType struct {
		schemabuilder.Union

		*Object
		*Object2
	}

	type enumType int32

	tests := []struct {
		name             string
		registrationFunc func(*schemabuilder.Schema) error
		query            string
		wantResultJSON   string
		wantError        string
		wantRuns         int64
	}{
		{
			name: "non-expensive run with single value",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) *Object {
					return object
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": { "key": "key1"}}
			]}
			`,
			wantRuns: 2, // Objects + Value
		},
		{
			name: "non-expensive run with multiple value",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) *Object {
					return object
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": { "key": "key1"}},
			{"key": "key2", "value": { "key": "key2"}}
			]}
			`,
			wantRuns: 2, // Objects + Value
		},
		{
			name: "expensive run with multiple value",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(ctx context.Context, object *Object) *Object {
					return object
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": { "key": "key1"}},
			{"key": "key2", "value": { "key": "key2"}}
			]}
			`,
			wantRuns: 3, // Objects + (Value * 2 objects)
		},
		{
			name: "non-expensive run with deep execution",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) *Object {
					return object
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						value {
							value {
								key
							}
						}
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": { "value": { "value": {"key": "key1"}}}},
			{"key": "key2", "value": { "value": { "value": {"key": "key2"}}}}
			]}
			`,
			wantRuns: 4, // Objects + Value + Value + Value
		},
		{
			name: "expensive run with deep execution",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(ctx context.Context, object *Object) *Object {
					return object
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						value {
							value {
								key
							}
						}
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": { "value": { "value": {"key": "key1"}}}},
			{"key": "key2", "value": { "value": { "value": {"key": "key2"}}}}
			]}
			`,
			wantRuns: 7, // Objects + ((Value + Value + Value) * 2 Objects)
		},
		{
			name: "non-expensive error",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) (*Object, error) {
					if object.Key == "key2" {
						return nil, errors.New("bad times")
					}
					return object, nil
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						key
					}
				}
			}`,
			wantError: "objects.1.value: bad times",
		},
		{
			name: "non-expensive error first index",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) (*Object, error) {
					if object.Key == "key1" {
						return nil, errors.New("bad times")
					}
					return object, nil
				})
				return nil
			},
			query: `
			{
				objects {
					key
					value {
						key
					}
				}
			}`,
			wantError: "objects.0.value: bad times",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := schemabuilder.NewSchema()

			require.NoError(t, tt.registrationFunc(builder))

			schema, err := builder.Build()
			require.NoError(t, err)

			q := graphql.MustParse(tt.query, nil)

			if err := graphql.PrepareQuery(context.Background(), schema.Query, q.SelectionSet); err != nil {
				t.Error(err)
			}

			c := &counterGoroutineScheduler{}
			e := graphql.NewExecutor(c)

			ctx := context.Background()
			res, err := e.Execute(ctx, schema.Query, nil, q)
			if tt.wantError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)

			wantParsedJSON := internal.ParseJSON(tt.wantResultJSON)
			gotJSON := internal.AsJSON(res)

			require.Equal(
				t,
				wantParsedJSON,
				gotJSON,
				"Mismatch for expected vs actual response.  Want:\n%s\nGot:\n%s",
				internal.MarshalJSON(wantParsedJSON),
				internal.MarshalJSON(gotJSON),
			)
			require.Equal(t, tt.wantRuns, c.count, "unexpected number of work units")
		})
	}
}

type counterGoroutineScheduler struct {
	wg    sync.WaitGroup
	count int64
}

func (q *counterGoroutineScheduler) Run(resolver graphql.UnitResolver, initialUnits ...*graphql.WorkUnit) {
	q.runEnqueue(resolver, initialUnits...)
	q.wg.Wait()
}

func (q *counterGoroutineScheduler) RunAll(ctx context.Context, executionUnits []*graphql.ExecutionUnit) {
}

func (q *counterGoroutineScheduler) runEnqueue(resolver graphql.UnitResolver, units ...*graphql.WorkUnit) {
	atomic.AddInt64(&q.count, int64(len(units)))
	for _, unit := range units {
		q.wg.Add(1)
		go func(u *graphql.WorkUnit) {
			defer q.wg.Done()
			units := resolver(u)
			q.runEnqueue(resolver, units...)
		}(unit)
	}
}

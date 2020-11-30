package graphql_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"fmt"


	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/thunder/batch"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/assert"
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
				}, schemabuilder.Expensive)
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
			name: "batch run with extra concurrency",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object {
					return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}, &Object{Key: "key3"}, &Object{Key: "key4"}, &Object{Key: "key5"}}
				})
				obj := schema.Object("Object", Object{})
				obj.BatchFieldFunc("value", func(ctx context.Context, objectBatch map[batch.Index]*Object) map[batch.Index]*Object {
					assert.True(t, len(objectBatch) > 0, "batch run with extra concurrency too few objects in batch")
					return objectBatch
				}, schemabuilder.NumParallelInvocationsFunc(func(ctx context.Context, numNodes int) int {
					assert.Equal(t, 5, numNodes, "batch run with extra concurrency invalid number of objects")
					return 2
				}))
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
			{"key": "key2", "value": { "key": "key2"}},
			{"key": "key3", "value": { "key": "key3"}},
			{"key": "key4", "value": { "key": "key4"}},
			{"key": "key5", "value": { "key": "key5"}}
			]}
			`,
			wantRuns: 3, // Objects + (Value * 2 batches of objects)
		},
		{
			name: "non-expensive run with extra concurrency",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object {
					return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}, &Object{Key: "key3"}, &Object{Key: "key4"}, &Object{Key: "key5"}}
				})
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(ctx context.Context, object *Object) *Object {
					return object
				}, schemabuilder.NumParallelInvocationsFunc(func(ctx context.Context, numNodes int) int {
					assert.Equal(t, 5, numNodes, "non-expensive run with extra concurrency invalid number of objects")
					return 2
				}))
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
			{"key": "key2", "value": { "key": "key2"}},
			{"key": "key3", "value": { "key": "key3"}},
			{"key": "key4", "value": { "key": "key4"}},
			{"key": "key5", "value": { "key": "key5"}}
			]}
			`,
			wantRuns: 3, // Objects + (Value * 2 batches of objects)
		},
		{
			name: "batch run with extremely high concurrency",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object {
					return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}, &Object{Key: "key3"}, &Object{Key: "key4"}, &Object{Key: "key5"}}
				})
				obj := schema.Object("Object", Object{})
				obj.BatchFieldFunc("value", func(ctx context.Context, objectBatch map[batch.Index]*Object) map[batch.Index]*Object {
					assert.True(t, len(objectBatch) > 0, "batch run with extremely high concurrency too few objects in batch")
					return objectBatch
				}, schemabuilder.NumParallelInvocationsFunc(func(ctx context.Context, numNodes int) int {
					assert.Equal(t, 5, numNodes, "batch run with extremely high concurrency invalid number of objects")
					return 10 // Bigger number than value passed in
				}))
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
			{"key": "key2", "value": { "key": "key2"}},
			{"key": "key3", "value": { "key": "key3"}},
			{"key": "key4", "value": { "key": "key4"}},
			{"key": "key5", "value": { "key": "key5"}}
			]}
			`,
			wantRuns: 6, // Objects + (Value * 5 batches of objects)
		},
		{
			name: "batch run with zero concurrency",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object {
					return []*Object{&Object{Key: "key1"}, &Object{Key: "key2"}, &Object{Key: "key3"}, &Object{Key: "key4"}, &Object{Key: "key5"}}
				})
				obj := schema.Object("Object", Object{})
				obj.BatchFieldFunc("value", func(ctx context.Context, objectBatch map[batch.Index]*Object) map[batch.Index]*Object {
					assert.True(t, len(objectBatch) > 0, "batch run with extremely high concurrency too few objects in batch")
					return objectBatch
				}, schemabuilder.NumParallelInvocationsFunc(func(ctx context.Context, numNodes int) int {
					assert.Equal(t, 5, numNodes, "batch run with extremely high concurrency invalid number of objects")
					return 0 // Invalid low value
				}))
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
			{"key": "key2", "value": { "key": "key2"}},
			{"key": "key3", "value": { "key": "key3"}},
			{"key": "key4", "value": { "key": "key4"}},
			{"key": "key5", "value": { "key": "key5"}}
			]}
			`,
			wantRuns: 2, // Objects + (Value * 1 batches of objects)
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
				}, schemabuilder.Expensive)
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
		{
			name: "non-expensive run with errorable directive",
			registrationFunc: func(schema *schemabuilder.Schema) error {
				schema.Query().FieldFunc("objects", func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} })
				obj := schema.Object("Object", Object{})
				obj.FieldFunc("value", func(object *Object) (*Object, error) {
					return nil, oops.Errorf("test error")
					// return object, nil
				})
				obj.FieldFunc("value2", func(object *Object) (*Object, error) {
					// return nil, oops.Errorf("test error")
					return object, nil
				})
				return nil
			},
			query: `
			{
				objects @errorable {
					key
					value @errorable {
						key
						value2 {
							key
						}
					}
					value2 @errorable {
						key
						value @errorable {
							key
						}
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": null, "value2": {"key":"key1","value":null}}
			]}
			`,
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
			res, errors, err := e.ExecuteWithPartialFailures(ctx, schema.Query, nil, q)
			fmt.Println("YPOOOO6", res, errors)
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
			if tt.wantRuns != 0 {
				require.Equal(t, tt.wantRuns, c.count, "unexpected number of work units")
			}
		
		})
	}
}

type counterGoroutineScheduler struct {
	wg sync.WaitGroup

	count int64
}

func (q *counterGoroutineScheduler) Run(resolver graphql.UnitResolver, initialUnits ...*graphql.WorkUnit) []error {
	errors := make([]error, 0, len(initialUnits))
	q.runEnqueue(resolver,  &errors, initialUnits...)

	q.wg.Wait()
	return errors
}

func (q *counterGoroutineScheduler) runEnqueue(resolver graphql.UnitResolver, errors *[]error, units ...*graphql.WorkUnit)  {
	atomic.AddInt64(&q.count, int64(len(units)))
	for _, unit := range units {
		q.wg.Add(1)
		go func(u *graphql.WorkUnit, errors *[]error) {
			defer q.wg.Done()
			units, unitErrors := resolver(u)
			*errors = append(*errors, unitErrors...)
			fmt.Println("YPPP", units, len(*errors), unitErrors)
			q.runEnqueue(resolver, errors, units...)
			// *errors = append(*errors, childrenErrors...)
			// return errors 
		}(unit, errors)
	}
}

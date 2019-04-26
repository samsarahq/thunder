package graphql_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/require"
)

func TestBatchFieldFuncExecution(t *testing.T) {
	type Object struct {
		Key string
	}
	tests := []struct {
		Name                  string
		GiveObjectFunc        interface{}
		GiveValueFunc         interface{}
		GiveValueFallbackFunc interface{}
		GiveQuery             string
		WantResultJSON        string
		WantError             string
	}{
		{
			Name:                  "good run with no value",
			GiveObjectFunc:        func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc:         func(ctx context.Context, o map[int]Object) (map[int]string, error) { return nil, nil },
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) { return nil, nil },
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value":null}
			]}
			`,
		},
		{
			Name:           "good run with response",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					myMap[idx] = "valfor" + val.Key
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			Name:           "panic run with response",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				panic("bad times")
				return nil, errors.New("my error here")
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				panic("bad times")
				return nil, errors.New("my error here")
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantError: "bad times",
		},
		{
			Name:           "error run with response",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				return nil, errors.New("my error here")
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				return nil, errors.New("my error here")
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantError: "my error here",
		},
		{
			Name:           "good run with pointer args",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]*Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					myMap[idx] = "valfor" + val.Key
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			Name:           "good run with pointer response type",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]*Object) (map[int]*string, error) {
				myMap := make(map[int]*string, len(o))
				for idx, val := range o {
					p := "valfor" + val.Key
					myMap[idx] = &p
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			Name:           "good run with pointer nil response type",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]*Object) (map[int]*string, error) {
				myMap := make(map[int]*string, len(o))
				for idx, _ := range o {
					myMap[idx] = nil
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				return nil, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": null}
			]}
			`,
		},
		{
			Name:           "run with all sub-object and args",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int]*Object, error) {
				myMap := make(map[int]*Object, len(o))
				for idx, val := range o {
					val.Key = args.Prefix + val.Key
					myMap[idx] = val
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }) (*Object, error) {
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}}
			]}
			`,
		},
		{
			Name:           "run with all possible parameters",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }, set *graphql.SelectionSet) (map[int]*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				myMap := make(map[int]*Object, len(o))
				for idx, val := range o {
					val.Key = args.Prefix + val.Key
					myMap[idx] = val
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }, set *graphql.SelectionSet) (*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}}
			]}
			`,
		},
		{
			Name: "run with lots of objects",
			GiveObjectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1"},
					{Key: "key2"},
					{Key: "key3"},
					{Key: "key4"},
					{Key: "key5"},
					{Key: "key6"},
					{Key: "key7"},
					{Key: "key8"},
				}
			},
			GiveValueFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }, set *graphql.SelectionSet) (map[int]*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				myMap := make(map[int]*Object, len(o))
				for idx, val := range o {
					val.Key = args.Prefix + val.Key
					myMap[idx] = val
				}
				return myMap, nil
			},
			GiveValueFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }, set *graphql.SelectionSet) (*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			GiveQuery: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			WantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}},
			{"key": "key2", "value": {"key": "testkey2"}},
			{"key": "key3", "value": {"key": "testkey3"}},
			{"key": "key4", "value": {"key": "testkey4"}},
			{"key": "key5", "value": {"key": "testkey5"}},
			{"key": "key6", "value": {"key": "testkey6"}},
			{"key": "key7", "value": {"key": "testkey7"}},
			{"key": "key8", "value": {"key": "testkey8"}}
			]}
			`,
		},
	}

	const (
		OldExecutor           = "OldExecutor"
		NewExecutorNoBatching = "NewExecutorNoBatching"
		NewExecutorBatching   = "NewExecutorBatching"
	)
	conditions := []string{OldExecutor, NewExecutorNoBatching, NewExecutorBatching}
	for _, tt := range tests {
		for _, cond := range conditions {
			t.Run(fmt.Sprintf("%s %s", tt.Name, cond), func(t *testing.T) {
				builder := schemabuilder.NewSchema()
				builder.Query().FieldFunc("objects", tt.GiveObjectFunc)

				obj := builder.Object("object", Object{})
				obj.BatchFieldFunc("value", tt.GiveValueFunc, tt.GiveValueFallbackFunc, func() bool {
					return cond == NewExecutorNoBatching
				})
				schema, err := builder.Build()
				require.NoError(t, err)

				q := graphql.MustParse(tt.GiveQuery, nil)

				if err := graphql.PrepareQuery(context.Background(), schema.Query, q.SelectionSet); err != nil {
					t.Error(err)
				}

				var e graphql.ExecutorRunner
				e = graphql.NewBatchExecutor(graphql.NewImmediateGoroutineScheduler())
				if cond == OldExecutor {
					e = &graphql.Executor{}
				}

				ctx := context.Background()
				res, err := e.Execute(ctx, schema.Query, nil, q)
				if tt.WantError != "" {
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.WantError)
					return
				}
				require.NoError(t, err)

				wantParsedJSON := internal.ParseJSON(tt.WantResultJSON)
				gotJSON := internal.AsJSON(res)

				require.True(
					t,
					reflect.DeepEqual(gotJSON, wantParsedJSON),
					"Mismatch for expected vs actual response.  Want:\n%s\nGot:\n%s",
					internal.MarshalJSON(wantParsedJSON),
					internal.MarshalJSON(gotJSON),
				)
			})
		}
	}
}

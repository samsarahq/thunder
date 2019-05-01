package graphql_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/stretchr/testify/require"
)

func TestBatchFieldFuncExecution(t *testing.T) {
	type Object struct {
		Key string
		Num int
	}
	tests := []struct {
		name                 string
		objectFunc           interface{}
		resolverFunc         interface{}
		resolverFallbackFunc interface{}
		query                string
		wantResultJSON       string
		wantError            string
	}{
		{
			name:                 "good run with no value",
			objectFunc:           func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc:         func(ctx context.Context, o map[int]Object) (map[int]string, error) { return nil, nil },
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) { return nil, nil },
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value":null}
			]}
			`,
		},
		{
			name:       "good run with response",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					myMap[idx] = "valfor" + val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			name:       "panic run with response",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				panic("bad times")
				return nil, errors.New("my error here")
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				panic("bad times")
				return nil, errors.New("my error here")
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantError: "bad times",
		},
		{
			name:       "error run with response",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
				return nil, errors.New("my error here")
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				return nil, errors.New("my error here")
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantError: "my error here",
		},
		{
			name:       "good run with pointer args",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					myMap[idx] = "valfor" + val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			name:       "good run with pointer response type",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]*string, error) {
				myMap := make(map[int]*string, len(o))
				for idx, val := range o {
					p := "valfor" + val.Key
					myMap[idx] = &p
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				str := "valfor" + o.Key
				return &str, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "valforkey1"}
			]}
			`,
		},
		{
			name:       "good run with pointer nil response type",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]*string, error) {
				myMap := make(map[int]*string, len(o))
				for idx, _ := range o {
					myMap[idx] = nil
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				return nil, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": null}
			]}
			`,
		},
		{
			name:       "run with all sub-object and args",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int]*Object, error) {
				myMap := make(map[int]*Object, len(o))
				for idx, val := range o {
					val.Key = args.Prefix + val.Key
					myMap[idx] = val
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }) (*Object, error) {
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			query: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}}
			]}
			`,
		},
		{
			name:       "run with all possible parameters",
			objectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }, set *graphql.SelectionSet) (map[int]*Object, error) {
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
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }, set *graphql.SelectionSet) (*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			query: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}}
			]}
			`,
		},
		{
			name: "run with lots of objects",
			objectFunc: func(ctx context.Context) []*Object {
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
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }, set *graphql.SelectionSet) (map[int]*Object, error) {
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
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }, set *graphql.SelectionSet) (*Object, error) {
				if set == nil {
					return nil, errors.New("Expected to have selectionSet")
				}
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			query: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			wantResultJSON: `
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
		{
			name: "run with lots of objects being filtered",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int]*Object, error) {
				myMap := make(map[int]*Object, len(o))
				for idx, val := range o {
					if val.Num%2 == 0 {
						continue
					}
					val.Key = args.Prefix + val.Key
					myMap[idx] = val
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }) (*Object, error) {
				if o.Num%2 == 0 {
					return nil, nil
				}
				o.Key = args.Prefix + o.Key
				return &o, nil
			},
			query: `
			{
				objects {
					key
					value(prefix: "test") {
						key
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": {"key": "testkey1"}},
			{"key": "key2", "value":null},
			{"key": "key3", "value": {"key": "testkey3"}},
			{"key": "key4", "value":null}
			]}
			`,
		},
		{
			name: "run with lots of string results being filtered",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int]*string, error) {
				myMap := make(map[int]*string, len(o))
				for idx, val := range o {
					if val.Num%2 == 0 {
						continue
					}
					val.Key = args.Prefix + val.Key
					myMap[idx] = &val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }) (*string, error) {
				if o.Num%2 == 0 {
					return nil, nil
				}
				o.Key = args.Prefix + o.Key
				return &o.Key, nil
			},
			query: `
			{
				objects {
					key
					value(prefix: "test")
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key1", "value": "testkey1"},
			{"key": "key2", "value":null},
			{"key": "key3", "value": "testkey3"},
			{"key": "key4", "value":null}
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
			t.Run(fmt.Sprintf("%s %s", tt.name, cond), func(t *testing.T) {
				builder := schemabuilder.NewSchema()
				builder.Query().FieldFunc("objects", tt.objectFunc)

				obj := builder.Object("object", Object{})
				obj.BatchFieldFuncWithFallback("value", tt.resolverFunc, tt.resolverFallbackFunc, func(ctx context.Context) bool {
					return cond == NewExecutorNoBatching
				})
				schema, err := builder.Build()
				require.NoError(t, err)

				q := graphql.MustParse(tt.query, nil)

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
			})
		}
	}
}

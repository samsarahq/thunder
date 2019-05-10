package graphql_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samsarahq/thunder/batch"
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
		name                 string
		objectFunc           interface{}
		resolverFunc         interface{}
		resolverFallbackFunc interface{}
		query                string
		markNonNullable      bool
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
		{
			name: "run with lots of string results being filtered, batch is not pointer",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					if val.Num%2 == 0 {
						continue
					}
					val.Key = args.Prefix + val.Key
					myMap[idx] = val.Key
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
		{
			name: "run with lots of list results being filtered",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			resolverFunc: func(ctx context.Context, o map[int]*Object, args struct{ Prefix string }) (map[int][]Object, error) {
				myMap := make(map[int][]Object, len(o))
				for idx, val := range o {
					if val.Num%2 == 0 {
						continue
					}
					val.Key = args.Prefix + val.Key
					list := []Object{*val}
					myMap[idx] = list
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object, args struct{ Prefix string }) ([]Object, error) {
				if o.Num%2 == 0 {
					return nil, nil
				}
				o.Key = args.Prefix + o.Key
				list := []Object{o}
				return list, nil
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
			{"key": "key1", "value": [{"key": "testkey1"}]},
			{"key": "key2", "value":[]},
			{"key": "key3", "value": [{"key": "testkey3"}]},
			{"key": "key4", "value":[]}
			]}
			`,
		},
		{
			name: "run with union type responses",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key0", Num: 0},
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]UnionType, error) {
				myMap := make(map[int]UnionType, len(o))
				for idx, val := range o {
					if val.Num == 0 {
						continue
					}
					if val.Num%2 != 0 {
						myMap[idx] = UnionType{Object: val}
						continue
					}
					myMap[idx] = UnionType{Object2: &Object2{Key2: val.Key}}
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*UnionType, error) {
				if o.Num == 0 {
					return nil, nil
				}
				if o.Num%2 != 0 {
					return &UnionType{Object: &o}, nil
				}
				return &UnionType{Object2: &Object2{Key2: o.Key}}, nil
			},
			query: `
			{
				objects {
					key
					value {
						__typename
						... on Object {key}
						... on Object2 {key2}
					}
				}
			}`,
			wantResultJSON: `
			{"objects": [
			{"key": "key0", "value": null},
			{"key": "key1", "value": {"__typename": "Object", "key": "key1"}},
			{"key": "key2", "value": {"__typename": "Object2", "key2": "key2"}},
			{"key": "key3", "value": {"__typename": "Object", "key": "key3"}},
			{"key": "key4", "value": {"__typename": "Object2", "key2": "key4"}}
			]}
			`,
		},
		{
			name: "run with enum non-nil resps",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			markNonNullable: true,
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]enumType, error) {
				myMap := make(map[int]enumType, len(o))
				for idx, val := range o {
					if val.Num%2 != 0 {
						myMap[idx] = enumType(1)
						continue
					}
					myMap[idx] = enumType(2)
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (enumType, error) {
				if o.Num%2 != 0 {
					return enumType(1), nil
				}
				return enumType(2), nil
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
			{"key": "key1", "value": "first"},
			{"key": "key2", "value": "second"},
			{"key": "key3", "value": "first"},
			{"key": "key4", "value": "second"}
			]}
			`,
		},
		{
			name: "run with string non-nil resps",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			markNonNullable: true,
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					myMap[idx] = val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (string, error) {
				return o.Key, nil
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
			{"key": "key1", "value": "key1"},
			{"key": "key2", "value": "key2"},
			{"key": "key3", "value": "key3"},
			{"key": "key4", "value": "key4"}
			]}
			`,
		},
		{
			name: "run with nil resp for non-nil endpoint",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
				}
			},
			markNonNullable: true,
			resolverFunc: func(ctx context.Context, o map[int]*Object) (map[int]string, error) {
				myMap := make(map[int]string, len(o))
				for idx, val := range o {
					if val.Num%2 != 0 {
						continue
					}
					myMap[idx] = val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (*string, error) {
				if o.Num%2 != 0 {
					return nil, nil
				}
				return &o.Key, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantError: "is marked non-nullable but returned a null value",
		},
		{
			name: "run with batch.Index",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					{Key: "key1", Num: 1},
					{Key: "key2", Num: 2},
					{Key: "key3", Num: 3},
					{Key: "key4", Num: 4},
				}
			},
			markNonNullable: true,
			resolverFunc: func(ctx context.Context, o map[batch.Index]*Object) (map[batch.Index]string, error) {
				myMap := make(map[batch.Index]string, len(o))
				for idx, val := range o {
					myMap[idx] = val.Key
				}
				return myMap, nil
			},
			resolverFallbackFunc: func(ctx context.Context, o Object) (string, error) {
				return o.Key, nil
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
			{"key": "key1", "value": "key1"},
			{"key": "key2", "value": "key2"},
			{"key": "key3", "value": "key3"},
			{"key": "key4", "value": "key4"}
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

				builder.Enum(enumType(0), map[string]enumType{
					"first":  enumType(1),
					"second": enumType(2),
					"third":  enumType(3),
				})

				_ = builder.Object("UnionType", UnionType{})
				_ = builder.Object("Object2", Object2{})
				obj := builder.Object("Object", Object{})

				options := make([]schemabuilder.FieldFuncOption, 0, 0)
				if tt.markNonNullable {
					options = append(options, schemabuilder.NonNullable)
				}
				obj.BatchFieldFuncWithFallback("value", tt.resolverFunc, tt.resolverFallbackFunc, func(ctx context.Context) bool {
					return cond == NewExecutorNoBatching
				}, options...)
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

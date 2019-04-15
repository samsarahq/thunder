package graphql_test

import (
	"context"
	"errors"
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
		Name           string
		GiveObjectFunc interface{}
		GiveValueFunc  interface{}
		GiveQuery      string
		WantResultJSON string
		WantError      string
	}{
		{
			Name:           "good run with no value",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc:  func(ctx context.Context, o map[int]Object) (map[int]string, error) { return nil, nil },
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
			Name:           "error run with response",
			GiveObjectFunc: func(ctx context.Context) []*Object { return []*Object{&Object{Key: "key1"}} },
			GiveValueFunc: func(ctx context.Context, o map[int]Object) (map[int]string, error) {
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
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			builder := schemabuilder.NewSchema()
			builder.Query().FieldFunc("objects", tt.GiveObjectFunc)

			obj := builder.Object("object", Object{})
			obj.BatchFieldFunc("value", tt.GiveValueFunc)
			schema, err := builder.Build()
			require.NoError(t, err)

			q := graphql.MustParse(tt.GiveQuery, nil)

			if err := graphql.PrepareQuery(schema.Query, q.SelectionSet); err != nil {
				t.Error(err)
			}
			e := graphql.Executor{}
			res, err := e.Execute(context.Background(), schema.Query, nil, q)
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

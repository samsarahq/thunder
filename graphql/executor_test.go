package graphql_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/internal"
	"github.com/samsarahq/thunder/internal/testgraphql"
	"github.com/stretchr/testify/require"
)

func makeQuery(onArgParse *func()) *graphql.Object {
	noArguments := func(json interface{}) (interface{}, error) {
		return nil, nil
	}

	query := &graphql.Object{
		Name:   "Query",
		Fields: make(map[string]*graphql.Field),
	}

	a := &graphql.Object{
		Name: "A",
		KeyField: &graphql.Field{
			Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
				return source, nil
			},
			Type: &graphql.Scalar{Type: "string"},
		},
		Fields: make(map[string]*graphql.Field),
	}

	query.Fields["a"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return 0, nil
		},
		Type:           a,
		ParseArguments: noArguments,
	}

	query.Fields["as"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return []int{0, 1, 2, 3}, nil
		},
		Type:           &graphql.List{Type: a},
		ParseArguments: noArguments,
	}

	query.Fields["static"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return "static", nil
		},
		Type:           &graphql.Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	query.Fields["error"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return nil, errors.New("test error")
		},
		Type:           &graphql.Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	query.Fields["panic"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			panic("test panic")
		},
		Type:           &graphql.Scalar{Type: "string"},
		ParseArguments: noArguments,
	}

	a.Fields["value"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return source.(int), nil
		},
		Type:           &graphql.Scalar{Type: "int"},
		ParseArguments: noArguments,
	}

	a.Fields["valuePtr"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			temp := source.(int)
			if temp%2 == 0 {
				return nil, nil
			}
			return &temp, nil
		},
		Type:           &graphql.Scalar{Type: "int"},
		ParseArguments: noArguments,
	}

	a.Fields["nested"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return source.(int) + 1, nil
		},
		Type:           a,
		ParseArguments: noArguments,
	}

	a.Fields["fieldWithArgs"] = &graphql.Field{
		Resolve: func(ctx context.Context, source, args interface{}, selectionSet *graphql.SelectionSet) (interface{}, error) {
			return 1, nil
		},
		Type: &graphql.Scalar{Type: "int"},
		ParseArguments: func(json interface{}) (interface{}, error) {
			if onArgParse != nil {
				(*onArgParse)()
			}
			return nil, nil
		},
	}

	return query
}

func TestBasic(t *testing.T) {
	query := makeQuery(nil)

	q := graphql.MustParse(`{
		static
		a { value nested { value } }
		as { value valuePtr }
	}`, nil)

	if err := graphql.PrepareQuery(context.Background(), query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := testgraphql.NewExecutorWrapper(t)
	result, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	// assert that result["as"][1]["valuePtr"] == 1 (and not a pointer to 1)
	root, _ := internal.AsJSON(result).(map[string]interface{})
	as, _ := root["as"].([]interface{})
	asObject, _ := as[1].(map[string]interface{})
	if int(asObject["valuePtr"].(float64)) != 1 {
		t.Error("Expected valuePtr to be 1, was", asObject["valuePtr"])
	}

	if !reflect.DeepEqual(internal.AsJSON(result), internal.ParseJSON(`
{
	"static": "static",
	"a": {
		"value": 0,
		"__key": 0,
		"nested": {
			"value": 1,
			"__key": 1
		}
	},
	"as": [
		{"value": 0, "valuePtr": null, "__key": 0},
		{"value": 1, "valuePtr": 1, "__key": 1},
		{"value": 2, "valuePtr": null, "__key": 2},
		{"value": 3, "valuePtr": 3, "__key": 3}
	]
}`)) {
		t.Error("bad value", spew.Sdump(internal.AsJSON(result)))
	}
}

func TestRepeatedFragment(t *testing.T) {
	ctr := 0
	countArgParse := func() {
		ctr++
	}
	query := makeQuery(&countArgParse)

	q := graphql.MustParse(`{
		static
		a { value nested { value ...frag } ...frag }
		as { value }
	}
	fragment frag on A {
		fieldWithArgs(arg1: 1)
	}
	`, nil)

	if err := graphql.PrepareQuery(context.Background(), query, q.SelectionSet); err != nil {
		t.Error(err)
	}
	e := testgraphql.NewExecutorWrapper(t)
	_, err := e.Execute(context.Background(), query, nil, q)
	if err != nil {
		t.Error(err)
	}

	if ctr != 1 {
		t.Errorf("Expected args for fragment to be parsed once, but they were parsed %d times.", ctr)
	}
}

/*
func TestMissingField(t *testing.T) {
	q := MustParse(`
		{
			unknown
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestMissingSelectors(t *testing.T) {
	q := MustParse(`
		{
			nested
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestUnwantedSelectors(t *testing.T) {
	q := MustParse(`
		{
			bar { bar }
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}

func TestBadArgs(t *testing.T) {
	q := MustParse(`
		{
			sum(a: "123", b: 4)
		}
	`, map[string]interface{}{})

	if err := PrepareQuery(query, q); err == nil {
		t.Error("expected error")
	}
}
*/

func TestError(t *testing.T) {
	query := makeQuery(nil)

	q := graphql.MustParse(`
		query foo {
			error
		}
	`, map[string]interface{}{})

	if err := graphql.PrepareQuery(context.Background(), query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := testgraphql.NewExecutorWrapper(t)
	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || err.Error() != "foo.error: test error" {
		t.Error("expected test error")
	}
}

// TestPanic tests that a panicing resolver will report an error to a
// context implementing PanicReporter instead of crashing the server.
func TestPanic(t *testing.T) {
	query := makeQuery(nil)

	q := graphql.MustParse(`
		{
			panic
		}
	`, nil)

	if err := graphql.PrepareQuery(context.Background(), query, q.SelectionSet); err != nil {
		t.Error(err)
	}

	e := testgraphql.NewExecutorWrapperWithoutExactErrorMatch(t)
	_, err := e.Execute(context.Background(), query, nil, q)
	if err == nil || !strings.Contains(err.Error(), "test panic") {
		t.Error("expected test panic")
	}
	if !strings.Contains(err.Error(), "executor_test.go") {
		t.Error("expected stacktrace")
	}
}

// TODO: Verify caching and concurrency

func TestExecutorRuns(t *testing.T) {
	type Object struct {
		Key string
	}
	tests := []struct {
		name           string
		objectFunc     interface{}
		resolverFunc   interface{}
		query          string
		wantResultJSON string
		wantError      string
	}{
		{
			name: "fail on 3rd value",
			objectFunc: func(ctx context.Context) []*Object {
				return []*Object{
					&Object{Key: "key1"},
					&Object{Key: "key2"},
					&Object{Key: "key3"},
				}
			},
			resolverFunc: func(ctx context.Context, o Object) (string, error) {
				if o.Key == "key3" {
					return "", errors.New("failing on third key")
				}
				return o.Key, nil
			},
			query: `
			{
				objects {
					key
					value
				}
			}`,
			wantError: "objects.2.value: failing on third key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := schemabuilder.NewSchema()
			builder.Query().FieldFunc("objects", tt.objectFunc)

			obj := builder.Object("object", Object{})
			obj.FieldFunc("value", tt.resolverFunc)
			schema, err := builder.Build()
			require.NoError(t, err)

			q := graphql.MustParse(tt.query, nil)

			if err := graphql.PrepareQuery(context.Background(), schema.Query, q.SelectionSet); err != nil {
				t.Error(err)
			}

			e := testgraphql.NewExecutorWrapper(t)

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

func Test_pathError_Reason(t *testing.T) {
	type fields struct {
		inner error
		path  []string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "empty list",
			fields: fields{
				inner: nil,
				path:  []string{},
			},
			want: "",
		},
		{
			name: "non empty list",
			fields: fields{
				inner: fmt.Errorf("error"),
				path:  []string{"a", "b", "c"},
			},
			want: "c.b.a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := graphql.PathErrorInit(tt.fields.inner, tt.fields.path).(*graphql.PathError)
			if got := pe.Reason(); got != tt.want {
				t.Errorf("Reason() = %v, want %v", got, tt.want)
			}
		})
	}
}
